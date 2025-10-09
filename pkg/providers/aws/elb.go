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
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// elbProvider is a provider for AWS Elastic Load Balancing (ELB) resources
type elbProvider struct {
	options   ProviderOptions
	elbClient *elb.ELB
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

func (ep *elbProvider) name() string {
	return "elb"
}

// GetResource returns all the resources in the store for a provider.
func (ep *elbProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range ep.regions.Regions {
		elbClients, ec2Clients := ep.getElbAndEc2Clients(region.RegionName)

		for index := range len(elbClients) {
			wg.Add(1)

			go func(elbClient *elb.ELB, ec2Client *ec2.EC2) {
				defer wg.Done()

				if resources, err := ep.listELBResources(elbClient, ec2Client); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(elbClients[index], ec2Clients[index])
		}
	}
	wg.Wait()
	return list, nil
}

func (ep *elbProvider) listELBResources(elbClient *elb.ELB, ec2Client *ec2.EC2) (*schema.Resources, error) {
	list := schema.NewResources()

	loadBalancerDescriptions, err := ep.getLoadBalancers(elbClient)
	if err != nil {
		return nil, err
	}

	for _, lb := range loadBalancerDescriptions {
		elbDNS := *lb.DNSName

		// Extract metadata for this load balancer
		var metadata map[string]string
		if ep.options.ExtendedMetadata {
			metadata = ep.getLoadBalancerMetadata(lb, elbClient)
		}

		resource := &schema.Resource{
			Provider: "aws",
			ID:       *lb.LoadBalancerName,
			DNSName:  elbDNS,
			Public:   true,
			Service:  ep.name(),
			Metadata: metadata,
		}
		list.Append(resource)

		if ep.elbClient == nil {
			continue
		}
		// Describe Instances for the Load Balancer
		for _, instance := range lb.Instances {
			instanceID := *instance.InstanceId
			instanceOutput, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
				InstanceIds: []*string{&instanceID},
			})
			if err != nil {
				return nil, err
			}
			// Extract private IP address
			for _, reservation := range instanceOutput.Reservations {
				for _, instance := range reservation.Instances {
					if instance.PrivateIpAddress != nil {
						// Extract metadata for target instance
						var instanceMetadata map[string]string
						if ep.options.ExtendedMetadata {
							instanceMetadata = ep.getTargetInstanceMetadata(instance, lb, reservation)
						}

						resource := &schema.Resource{
							Provider:    "aws",
							ID:          instanceID,
							PrivateIpv4: *instance.PrivateIpAddress,
							Public:      false,
							Service:     ep.name(),
							Metadata:    instanceMetadata,
						}
						list.Append(resource)
					}
				}
			}
		}
	}

	return list, nil
}

func (ep *elbProvider) getLoadBalancers(elbClient *elb.ELB) ([]*elb.LoadBalancerDescription, error) {
	var loadBalancers []*elb.LoadBalancerDescription
	req := &elb.DescribeLoadBalancersInput{}
	for {
		lbOutput, err := elbClient.DescribeLoadBalancers(req)
		if err != nil {
			return nil, err
		}
		loadBalancers = append(loadBalancers, lbOutput.LoadBalancerDescriptions...)
		if aws.StringValue(lbOutput.NextMarker) == "" {
			break
		}
		req.SetMarker(aws.StringValue(lbOutput.NextMarker))
	}
	return loadBalancers, nil
}

func (ep *elbProvider) getElbAndEc2Clients(region *string) ([]*elb.ELB, []*ec2.EC2) {
	elbClients := make([]*elb.ELB, 0)
	ec2Clients := make([]*ec2.EC2, 0)

	albClient := elb.New(ep.session, aws.NewConfig().WithRegion(*region))
	elbClients = append(elbClients, albClient)

	ec2Client := ec2.New(ep.session, aws.NewConfig().WithRegion(*region))
	ec2Clients = append(ec2Clients, ec2Client)

	if ep.options.AssumeRoleName == "" || len(ep.options.AccountIds) < 1 {
		return elbClients, ec2Clients
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
		elbClients = append(elbClients, elb.New(assumeSession))
		ec2Clients = append(ec2Clients, ec2.New(assumeSession))
	}
	return elbClients, ec2Clients
}

func (ep *elbProvider) getLoadBalancerMetadata(lb *elb.LoadBalancerDescription, elbClient *elb.ELB) map[string]string {
	metadata := make(map[string]string)

	// Basic load balancer information
	schema.AddMetadata(metadata, "load_balancer_name", lb.LoadBalancerName)
	schema.AddMetadata(metadata, "dns_name", lb.DNSName)
	schema.AddMetadata(metadata, "hosted_zone_name", lb.CanonicalHostedZoneName)
	schema.AddMetadata(metadata, "hosted_zone_id", lb.CanonicalHostedZoneNameID)
	schema.AddMetadata(metadata, "scheme", lb.Scheme)
	schema.AddMetadata(metadata, "vpc_id", lb.VPCId)

	if lb.CreatedTime != nil {
		metadata["created_time"] = lb.CreatedTime.Format(time.RFC3339)
	}

	schema.AddMetadataList(metadata, "availability_zones", lb.AvailabilityZones)
	schema.AddMetadataList(metadata, "subnet_ids", lb.Subnets)
	schema.AddMetadataList(metadata, "security_groups", lb.SecurityGroups)

	// Instances
	schema.AddMetadataInt(metadata, "instance_count", len(lb.Instances))
	if len(lb.Instances) > 0 {
		var instanceIds []string
		for _, instance := range lb.Instances {
			if instance.InstanceId != nil {
				instanceIds = append(instanceIds, aws.StringValue(instance.InstanceId))
			}
		}
		if len(instanceIds) > 0 {
			metadata["instance_ids"] = strings.Join(instanceIds, ",")
		}
	}

	// Listener descriptions
	schema.AddMetadataInt(metadata, "listener_count", len(lb.ListenerDescriptions))
	if len(lb.ListenerDescriptions) > 0 {
		var ports []string
		var protocols []string
		for _, listener := range lb.ListenerDescriptions {
			if listener.Listener != nil {
				if listener.Listener.LoadBalancerPort != nil {
					ports = append(ports, fmt.Sprintf("%d", aws.Int64Value(listener.Listener.LoadBalancerPort)))
				}
				if listener.Listener.Protocol != nil {
					protocols = append(protocols, aws.StringValue(listener.Listener.Protocol))
				}
			}
		}
		if len(ports) > 0 {
			metadata["listener_ports"] = strings.Join(ports, ",")
		}
		if len(protocols) > 0 {
			metadata["listener_protocols"] = strings.Join(protocols, ",")
		}
	}

	// Backend server descriptions
	schema.AddMetadataInt(metadata, "backend_server_count", len(lb.BackendServerDescriptions))

	// Get tags
	if lb.LoadBalancerName != nil {
		if tagOutput, err := elbClient.DescribeTags(&elb.DescribeTagsInput{
			LoadBalancerNames: []*string{lb.LoadBalancerName},
		}); err == nil && tagOutput.TagDescriptions != nil && len(tagOutput.TagDescriptions) > 0 {
			for _, tagDesc := range tagOutput.TagDescriptions {
				if tagString := buildELBTagString(tagDesc.Tags); tagString != "" {
					metadata["tags"] = tagString
					break
				}
			}
		}
	}

	return metadata
}

func (ep *elbProvider) getTargetInstanceMetadata(instance *ec2.Instance, lb *elb.LoadBalancerDescription, reservation *ec2.Reservation) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "instance_id", instance.InstanceId)
	schema.AddMetadata(metadata, "instance_type", instance.InstanceType)
	schema.AddMetadata(metadata, "private_dns_name", instance.PrivateDnsName)
	schema.AddMetadata(metadata, "public_dns_name", instance.PublicDnsName)
	schema.AddMetadata(metadata, "subnet_id", instance.SubnetId)
	schema.AddMetadata(metadata, "vpc_id", instance.VpcId)

	if instance.State != nil {
		schema.AddMetadata(metadata, "instance_state", instance.State.Name)
	}
	if instance.Placement != nil {
		schema.AddMetadata(metadata, "availability_zone", instance.Placement.AvailabilityZone)
	}
	schema.AddMetadata(metadata, "owner_id", reservation.OwnerId)
	schema.AddMetadata(metadata, "load_balancer_name", lb.LoadBalancerName)
	schema.AddMetadata(metadata, "load_balancer_dns", lb.DNSName)

	if len(instance.Tags) > 0 {
		if tagString := buildTagString(instance.Tags); tagString != "" {
			metadata["instance_tags"] = tagString
		}
	}

	if len(instance.SecurityGroups) > 0 {
		var sgIds, sgNames []string
		for _, sg := range instance.SecurityGroups {
			if sg.GroupId != nil {
				sgIds = append(sgIds, aws.StringValue(sg.GroupId))
			}
			if sg.GroupName != nil {
				sgNames = append(sgNames, aws.StringValue(sg.GroupName))
			}
		}
		if len(sgIds) > 0 {
			metadata["security_group_ids"] = strings.Join(sgIds, ",")
		}
		if len(sgNames) > 0 {
			metadata["security_group_names"] = strings.Join(sgNames, ",")
		}
	}

	return metadata
}

func buildELBTagString(tags []*elb.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
