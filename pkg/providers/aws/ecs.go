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
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(regionName)},
		)
		if err != nil {
			return nil, errors.Wrapf(err, "could not create session for region %s", regionName)
		}
		ecsClient := ecs.New(sess)
		err = listECSResources(ecsClient, list, sess)
		if err != nil {
			return nil, errors.Wrapf(err, "could not list ECS resources for region %s", regionName)
		}
	}
	return list, nil
}

func listECSResources(ecsClient *ecs.ECS, list *schema.Resources, sess *session.Session) error {
	clustersOutput, err := ecsClient.ListClusters(&ecs.ListClustersInput{})
	if err != nil {
		return errors.Wrap(err, "could not list clusters")
	}

	for _, clusterArn := range clustersOutput.ClusterArns {
		// List services in the cluster
		servicesOutput, err := ecsClient.ListServices(&ecs.ListServicesInput{
			Cluster: clusterArn,
		})
		if err != nil {
			return errors.Wrap(err, "could not list services")
		}

		for _, serviceArn := range servicesOutput.ServiceArns {
			// List tasks in the service
			tasksOutput, err := ecsClient.ListTasks(&ecs.ListTasksInput{
				Cluster:     clusterArn,
				ServiceName: serviceArn,
			})
			if err != nil {
				return errors.Wrap(err, "could not list tasks")
			}

			if len(tasksOutput.TaskArns) == 0 {
				continue
			}

			describeTasksOutput, err := ecsClient.DescribeTasks(&ecs.DescribeTasksInput{
				Cluster: clusterArn,
				Tasks:   tasksOutput.TaskArns,
			})
			if err != nil {
				return errors.Wrap(err, "could not describe tasks")
			}

			for _, task := range describeTasksOutput.Tasks {
				for _, attachment := range task.Attachments {
					for _, detail := range attachment.Details {
						if *detail.Name == "networkInterfaceId" {
							networkInterfaceId := *detail.Value
							ec2Client := ec2.New(sess)
							networkInterfaceOutput, err := ec2Client.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
								NetworkInterfaceIds: []*string{aws.String(networkInterfaceId)},
							})
							if err != nil {
								return errors.Wrap(err, "could not describe network interface")
							}

							for _, networkInterface := range networkInterfaceOutput.NetworkInterfaces {
								resource := &schema.Resource{
									Provider:    "aws",
									ID:          networkInterfaceId,
									PublicIPv4:  aws.StringValue(networkInterface.Association.PublicIp),
									PrivateIpv4: aws.StringValue(networkInterface.PrivateIpAddress),
									DNSName:     aws.StringValue(networkInterface.Association.PublicDnsName),
									Public:      aws.StringValue(networkInterface.Association.PublicIp) != "",
								}
								list.Append(resource)
							}
						}
					}
				}
			}
		}
	}
	return nil
}
