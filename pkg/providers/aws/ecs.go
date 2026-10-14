package aws

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
	var errs []error

	for _, region := range ep.regions.Regions {
		ecsClients, ec2Clients := ep.getEcsAndEc2Clients(region.RegionName)
		for index := range len(ecsClients) {
			wg.Add(1)

			go func(ecsClient *ecs.ECS, ec2Client *ec2.EC2) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						mu.Lock()
						errs = append(errs, fmt.Errorf("panic in ecs provider: %v", r))
						mu.Unlock()
					}
				}()
				if resources, err := ep.listECSResources(ecsClient, ec2Client); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(ecsClients[index], ec2Clients[index])
		}
	}
	wg.Wait()
	if len(errs) > 0 && len(list.Items) == 0 {
		return nil, fmt.Errorf("ecs: all workers failed: %v", errs)
	}
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
								// Handle Fargate tasks - they don't have container instances but have ENIs
								if err := ep.processFargateTask(task, ec2Client, list); err != nil {
									continue
								}
							} else {
								// Handle EC2-based ECS tasks
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

											var metadata map[string]string
											if ep.options.ExtendedMetadata {
												metadata = ep.getECSTaskMetadata(task, instance, reservation, clusterArn, serviceArn)
											}

											if privateIp4 != "" {
												resource := &schema.Resource{
													ID:          aws.StringValue(instance.InstanceId),
													Provider:    "aws",
													PrivateIpv4: privateIp4,
													Public:      false,
													Service:     ep.name(),
													Metadata:    metadata,
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
													Metadata:   metadata,
												}
												list.Append(resource)
											}
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

func (ep *ecsProvider) processFargateTask(task *ecs.Task, ec2Client *ec2.EC2, list *schema.Resources) error {
	for _, attachment := range task.Attachments {
		if aws.StringValue(attachment.Type) != "ElasticNetworkInterface" {
			continue
		}

		var eniID *string
		for _, detail := range attachment.Details {
			if aws.StringValue(detail.Name) == "networkInterfaceId" {
				eniID = detail.Value
				break
			}
		}

		if eniID == nil {
			continue
		}

		describeNetworkInterfacesInput := &ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: []*string{eniID},
		}

		describeNetworkInterfacesOutput, err := ec2Client.DescribeNetworkInterfaces(describeNetworkInterfacesInput)
		if err != nil {
			return errors.Wrap(err, "could not describe network interfaces")
		}

		for _, eni := range describeNetworkInterfacesOutput.NetworkInterfaces {
			taskID := aws.StringValue(task.TaskArn)
			if task.TaskArn != nil {
				if arnComponents := parseARN(*task.TaskArn); arnComponents != nil {
					taskID = arnComponents.GetResourceName()
				}
			}

			var metadata map[string]string
			if ep.options.ExtendedMetadata {
				var clusterArn, serviceArn *string
				if task.ClusterArn != nil {
					clusterArn = task.ClusterArn
				}
				if task.Group != nil && strings.HasPrefix(*task.Group, "service:") {
					serviceName := strings.TrimPrefix(*task.Group, "service:")
					serviceArn = aws.String(serviceName)
				}
				metadata = ep.getFargateTaskMetadata(task, eni, clusterArn, serviceArn)
			}

			privateIp4 := aws.StringValue(eni.PrivateIpAddress)
			if privateIp4 != "" {
				resource := &schema.Resource{
					ID:          taskID,
					Provider:    "aws",
					PrivateIpv4: privateIp4,
					Public:      false,
					Service:     ep.name(),
					Metadata:    metadata,
				}
				list.Append(resource)
			}

			if eni.Association != nil {
				publicIp4 := aws.StringValue(eni.Association.PublicIp)
				if publicIp4 != "" {
					resource := &schema.Resource{
						ID:         taskID,
						Provider:   "aws",
						PublicIPv4: publicIp4,
						Public:     true,
						Service:    ep.name(),
						Metadata:   metadata,
					}
					list.Append(resource)
				}
			}
		}
	}
	return nil
}

func (ep *ecsProvider) getECSTaskMetadata(task *ecs.Task, instance *ec2.Instance, reservation *ec2.Reservation, clusterArn, serviceArn *string) map[string]string {
	metadata := make(map[string]string)

	if task.TaskArn != nil {
		taskArn := aws.StringValue(task.TaskArn)
		metadata["task_arn"] = taskArn

		if arnComponents := parseARN(taskArn); arnComponents != nil {
			metadata["task_id"] = arnComponents.GetResourceName()
			if arnComponents.AccountID != "" {
				metadata["owner_id"] = arnComponents.AccountID
			}
		}
	}

	if reservation != nil {
		schema.AddMetadata(metadata, "instance_owner_id", reservation.OwnerId)
	}

	if clusterArn != nil {
		clusterArnStr := aws.StringValue(clusterArn)
		metadata["cluster_arn"] = clusterArnStr

		if arnComponents := parseARN(clusterArnStr); arnComponents != nil {
			metadata["cluster_name"] = arnComponents.GetResourceName()
		}
	}

	if serviceArn != nil {
		serviceArnStr := aws.StringValue(serviceArn)
		metadata["service_arn"] = serviceArnStr

		if arnComponents := parseARN(serviceArnStr); arnComponents != nil {
			metadata["service_name"] = arnComponents.GetResourceName()
		}
	}

	schema.AddMetadata(metadata, "task_definition", task.TaskDefinitionArn)

	schema.AddMetadata(metadata, "task_status", task.LastStatus)
	schema.AddMetadata(metadata, "desired_status", task.DesiredStatus)
	schema.AddMetadata(metadata, "launch_type", task.LaunchType)
	schema.AddMetadata(metadata, "platform_version", task.PlatformVersion)

	schema.AddMetadata(metadata, "cpu", task.Cpu)
	schema.AddMetadata(metadata, "memory", task.Memory)

	schema.AddMetadata(metadata, "container_instance_arn", task.ContainerInstanceArn)

	schema.AddMetadata(metadata, "availability_zone", task.AvailabilityZone)

	if task.CreatedAt != nil {
		metadata["created_at"] = task.CreatedAt.Format(time.RFC3339)
	}
	if task.StartedAt != nil {
		metadata["started_at"] = task.StartedAt.Format(time.RFC3339)
	}

	if instance != nil {
		schema.AddMetadata(metadata, "instance_id", instance.InstanceId)
		schema.AddMetadata(metadata, "instance_type", instance.InstanceType)
		schema.AddMetadata(metadata, "vpc_id", instance.VpcId)
		schema.AddMetadata(metadata, "subnet_id", instance.SubnetId)

		if len(instance.Tags) > 0 {
			if tagString := buildEC2TagString(instance.Tags); tagString != "" {
				metadata["instance_tags"] = tagString
			}
		}
	}

	if len(task.Tags) > 0 {
		if tagString := buildECSTagString(task.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	if len(task.Containers) > 0 {
		var containerNames []string
		for _, container := range task.Containers {
			if container.Name != nil {
				containerNames = append(containerNames, aws.StringValue(container.Name))
			}
		}
		if len(containerNames) > 0 {
			metadata["container_names"] = strings.Join(containerNames, ",")
		}
		schema.AddMetadataInt(metadata, "container_count", len(task.Containers))
	}

	schema.AddMetadata(metadata, "group", task.Group)

	schema.AddMetadata(metadata, "connectivity", task.Connectivity)

	schema.AddMetadata(metadata, "health_status", task.HealthStatus)

	return metadata
}

func (ep *ecsProvider) getFargateTaskMetadata(task *ecs.Task, eni *ec2.NetworkInterface, clusterArn, serviceArn *string) map[string]string {
	metadata := make(map[string]string)

	baseMetadata := ep.getECSTaskMetadata(task, nil, nil, clusterArn, serviceArn)
	for k, v := range baseMetadata {
		metadata[k] = v
	}

	metadata["launch_type"] = "FARGATE"

	if eni != nil {
		schema.AddMetadata(metadata, "network_interface_id", eni.NetworkInterfaceId)
		schema.AddMetadata(metadata, "vpc_id", eni.VpcId)
		schema.AddMetadata(metadata, "subnet_id", eni.SubnetId)
		schema.AddMetadata(metadata, "availability_zone", eni.AvailabilityZone)

		if len(eni.Groups) > 0 {
			var sgIds []string
			for _, sg := range eni.Groups {
				if sg.GroupId != nil {
					sgIds = append(sgIds, aws.StringValue(sg.GroupId))
				}
			}
			if len(sgIds) > 0 {
				metadata["security_groups"] = strings.Join(sgIds, ",")
			}
		}
	}

	return metadata
}

func buildECSTagString(tags []*ecs.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}

func buildEC2TagString(tags []*ec2.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
