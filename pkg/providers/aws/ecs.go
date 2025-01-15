package aws

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// ecsProvider is a provider for aws ecs API
type ecsProvider struct {
	options   ProviderOptions
	ecsClient *ecs.ECS
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

func (ep *ecsProvider) name() string {
	return "ecs"
}

// GetResource returns all the resources in the store for a provider.
func (ep *ecsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range ep.regions.Regions {
		ecsCleints, ec2Clients := ep.getEcsAndEc2Clients(region.RegionName)
		for index := range len(ecsCleints) {
			wg.Add(1)

			go func(ecsClient *ecs.ECS, ec2Client *ec2.EC2) {
				defer wg.Done()
				if resources, err := ep.listECSResources(ecsClient, ec2Client); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(ecsCleints[index], ec2Clients[index])
		}
	}
	wg.Wait()
	return list, nil
}

func (ep *ecsProvider) listECSResources(ecsClient *ecs.ECS, ec2Client *ec2.EC2) (*schema.Resources, error) {
	list := schema.NewResources()
	req := &ecs.ListClustersInput{
		MaxResults: aws.Int64(100),
	}
	for {
		clustersOutput, err := ecsClient.ListClusters(req)
		if err != nil {
			return nil, err
		}

		for _, clusterArn := range clustersOutput.ClusterArns {
			listServicesInputReq := &ecs.ListServicesInput{
				Cluster:    clusterArn,
				MaxResults: aws.Int64(100),
			}
			for {
				servicesOutput, err := ecsClient.ListServices(listServicesInputReq)
				if err != nil {
					return nil, errors.Wrap(err, "could not list ECS services")
				}

				for _, serviceArn := range servicesOutput.ServiceArns {
					listTasksInputReq := &ecs.ListTasksInput{
						Cluster:     clusterArn,
						ServiceName: serviceArn,
						MaxResults:  aws.Int64(100),
					}

					for {
						tasksOutput, err := ecsClient.ListTasks(listTasksInputReq)
						if err != nil {
							return nil, errors.Wrap(err, "could not list tasks")
						}
						if len(tasksOutput.TaskArns) == 0 {
							break
						}
						describeTasksInput := &ecs.DescribeTasksInput{
							Cluster: clusterArn,
							Tasks:   tasksOutput.TaskArns,
						}

						describeTasksOutput, err := ecsClient.DescribeTasks(describeTasksInput)
						if err != nil {
							return nil, errors.Wrap(err, "could not describe tasks")
						}

						for _, task := range describeTasksOutput.Tasks {
							if task.ContainerInstanceArn == nil {
								continue
							}
							describeContainerInstancesInput := &ecs.DescribeContainerInstancesInput{
								Cluster:            clusterArn,
								ContainerInstances: []*string{task.ContainerInstanceArn},
							}

							describeContainerInstancesOutput, err := ecsClient.DescribeContainerInstances(describeContainerInstancesInput)
							if err != nil {
								return nil, errors.Wrap(err, "could not describe container instances")
							}

							for _, containerInstance := range describeContainerInstancesOutput.ContainerInstances {
								instanceID := containerInstance.Ec2InstanceId
								describeInstancesInput := &ec2.DescribeInstancesInput{
									InstanceIds: []*string{instanceID},
								}

								describeInstancesOutput, err := ec2Client.DescribeInstances(describeInstancesInput)
								if err != nil {
									continue
								}

								for _, reservation := range describeInstancesOutput.Reservations {
									for _, instance := range reservation.Instances {
										ip4 := aws.StringValue(instance.PublicIpAddress)
										ip6 := aws.StringValue(instance.Ipv6Address)
										privateIp4 := aws.StringValue(instance.PrivateIpAddress)

										if privateIp4 != "" {
											resource := &schema.Resource{
												ID:          aws.StringValue(instance.InstanceId),
												Provider:    "aws",
												PrivateIpv4: privateIp4,
												Public:      false,
												Service:     ep.name(),
											}
											list.Append(resource)
										}

										if ip4 != "" || ip6 != "" {
											resource := &schema.Resource{
												ID:         aws.StringValue(instance.InstanceId),
												Provider:   "aws",
												PublicIPv4: ip4,
												PublicIPv6: ip6,
												Public:     true,
												Service:    ep.name(),
											}
											list.Append(resource)
										}
									}
								}
							}
						}

						if aws.StringValue(listTasksInputReq.NextToken) == "" {
							break
						}
						listTasksInputReq.SetNextToken(*listTasksInputReq.NextToken)
					}
				}

				if aws.StringValue(servicesOutput.NextToken) == "" {
					break
				}
				listServicesInputReq.SetNextToken(*servicesOutput.NextToken)
			}
		}
		if aws.StringValue(clustersOutput.NextToken) == "" {
			break
		}
		req.SetNextToken(*clustersOutput.NextToken)
	}
	return list, nil
}

func (ep *ecsProvider) getEcsAndEc2Clients(region *string) ([]*ecs.ECS, []*ec2.EC2) {
	ecsClients := make([]*ecs.ECS, 0)
	ec2Clients := make([]*ec2.EC2, 0)

	albClient := ecs.New(ep.session, aws.NewConfig().WithRegion(*region))
	ecsClients = append(ecsClients, albClient)

	ec2Client := ec2.New(ep.session, aws.NewConfig().WithRegion(*region))
	ec2Clients = append(ec2Clients, ec2Client)

	if ep.options.AssumeRoleName == "" || len(ep.options.AccountIds) < 1 {
		return ecsClients, ec2Clients
	}

	for _, accountId := range ep.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, ep.options.AssumeRoleName)
		creds := stscreds.NewCredentials(ep.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      region,
			Credentials: creds,
		})
		if err != nil {
			continue
		}
		ecsClients = append(ecsClients, ecs.New(assumeSession))
		ec2Clients = append(ec2Clients, ec2.New(assumeSession))
	}
	return ecsClients, ec2Clients
}
