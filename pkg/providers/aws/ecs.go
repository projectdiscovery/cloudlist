package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// ecsProvider is a provider for aws ecs API
type ecsProvider struct {
	id        string
	ecsClient *ecs.ECS
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

// GetResource returns all the resources in the store for a provider.
func (ep *ecsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, region := range ep.regions.Regions {
		regionName := *region.RegionName
		ecsClient := ecs.New(ep.session, aws.NewConfig().WithRegion(regionName))
		ec2Client := ec2.New(ep.session, aws.NewConfig().WithRegion(regionName))
		_ = listECSResources(ecsClient, ec2Client, list)
	}
	return list, nil
}

func listECSResources(ecsClient *ecs.ECS, ec2Client *ec2.EC2, list *schema.Resources) error {
	req := &ecs.ListClustersInput{
		MaxResults: aws.Int64(100),
	}
	for {
		clustersOutput, err := ecsClient.ListClusters(req)
		if err != nil {
			return errors.Wrap(err, "could not list ECS clusters")
		}

		for _, clusterArn := range clustersOutput.ClusterArns {
			listServicesInputReq := &ecs.ListServicesInput{
				Cluster:    clusterArn,
				MaxResults: aws.Int64(100),
			}
			for {
				servicesOutput, err := ecsClient.ListServices(listServicesInputReq)
				if err != nil {
					return errors.Wrap(err, "could not list ECS services")
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
							return errors.Wrap(err, "could not list tasks")
						}

						if len(tasksOutput.TaskArns) == 0 {
							continue
						}

						describeTasksInput := &ecs.DescribeTasksInput{
							Cluster: clusterArn,
							Tasks:   tasksOutput.TaskArns,
						}

						describeTasksOutput, err := ecsClient.DescribeTasks(describeTasksInput)
						if err != nil {
							return errors.Wrap(err, "could not describe tasks")
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
								return errors.Wrap(err, "could not describe container instances")
							}

							for _, containerInstance := range describeContainerInstancesOutput.ContainerInstances {
								instanceID := containerInstance.Ec2InstanceId
								describeInstancesInput := &ec2.DescribeInstancesInput{
									InstanceIds: []*string{instanceID},
								}

								describeInstancesOutput, err := ec2Client.DescribeInstances(describeInstancesInput)
								if err != nil {
									return errors.Wrap(err, "could not describe EC2 instances")
								}

								for _, reservation := range describeInstancesOutput.Reservations {
									for _, instance := range reservation.Instances {
										privateIP := aws.StringValue(instance.PrivateIpAddress)
										publicIP := aws.StringValue(instance.PublicIpAddress)

										if privateIP != "" {
											resource := &schema.Resource{
												ID:          aws.StringValue(instance.InstanceId),
												Provider:    "aws",
												PrivateIpv4: privateIP,
												Public:      false,
											}
											list.Append(resource)
											// fmt.Printf("Private Resource: %+v\n", resource)
										}

										if publicIP != "" {
											resource := &schema.Resource{
												ID:         aws.StringValue(instance.InstanceId),
												Provider:   "aws",
												PublicIPv4: publicIP,
												Public:     true,
											}
											list.Append(resource)
											// fmt.Printf("Public Resource: %+v\n", resource)
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
	return nil
}
