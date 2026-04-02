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
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// elbV2Provider is a provider for AWS Application Load Balancing (ELBV2) resources
type elbV2Provider struct {
	options   ProviderOptions
	albClient *elbv2.ELBV2
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

func (ep *elbV2Provider) name() string {
	return "alb"
}

// GetResource returns all the resources in the store for a provider.
func (ep *elbV2Provider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, region := range ep.regions.Regions {
		albClients, ec2Clients := ep.getElbV2AndEc2Clients(region.RegionName)
		for index := range len(albClients) {
			wg.Add(1)

			go func(albClient *elbv2.ELBV2, ec2Client *ec2.EC2) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						mu.Lock()
						errs = append(errs, fmt.Errorf("panic in alb provider: %v", r))
						mu.Unlock()
					}
				}()

				if resources, err := ep.listELBV2Resources(albClient, ec2Client); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(albClients[index], ec2Clients[index])
		}
	}
	wg.Wait()
	if len(errs) > 0 && len(list.Items) == 0 {
		return nil, fmt.Errorf("alb: all workers failed: %v", errs)
	}
	return list, nil
}

func (ep *elbV2Provider) listELBV2Resources(albClient *elbv2.ELBV2, ec2Client *ec2.EC2) (*schema.Resources, error) {
	list := schema.NewResources()

	loadBalancers, err := ep.getLoadBalancers(albClient)
	if err != nil {
		return nil, err
	}

	for _, lb := range loadBalancers {
		if lb.DNSName == nil || lb.LoadBalancerName == nil {
			continue
		}
		albDNS := *lb.DNSName

		// Extract metadata for this load balancer
		var metadata map[string]string
		if ep.options.ExtendedMetadata {
			metadata = ep.getLoadBalancerMetadata(lb, albClient)
		}

		resource := &schema.Resource{
			Provider: "aws",
			ID:       *lb.LoadBalancerName,
			DNSName:  albDNS,
			Public:   true,
			Service:  ep.name(),
			Metadata: metadata,
		}
		list.Append(resource)

		if ec2Client == nil {
			continue
		}
		// Describe targets for the Load Balancer
		targetsOutput, err := albClient.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
			LoadBalancerArn: lb.LoadBalancerArn,
		})
		if err != nil {
			continue
		}

		for _, tg := range targetsOutput.TargetGroups {
			targets, err := albClient.DescribeTargetHealth(&elbv2.DescribeTargetHealthInput{
				TargetGroupArn: tg.TargetGroupArn,
			})
			if err != nil {
				continue
			}

			for _, target := range targets.TargetHealthDescriptions {
				if target.Target == nil || target.Target.Id == nil {
					continue
				}
				instanceID := *target.Target.Id
				instanceOutput, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
					InstanceIds: []*string{&instanceID},
				})
				if err != nil {
					return nil, errors.Wrapf(err, "could not describe instance %s", instanceID)
				}
				// Extract private IP address
				for _, reservation := range instanceOutput.Reservations {
					for _, instance := range reservation.Instances {
						if instance.PrivateIpAddress != nil {
							// Extract metadata for target instance
							var targetMetadata map[string]string
							if ep.options.ExtendedMetadata {
								targetMetadata = ep.getTargetInstanceMetadata(instance, target, tg, lb)
							}

							resource := &schema.Resource{
								Provider:    "aws",
								ID:          instanceID,
								PrivateIpv4: *instance.PrivateIpAddress,
								Public:      false,
								Service:     ep.name(),
								Metadata:    targetMetadata,
							}
							list.Append(resource)
						}
					}
				}
			}
		}
	}
	return list, nil
}

func (ep *elbV2Provider) getLoadBalancers(albClient *elbv2.ELBV2) ([]*elbv2.LoadBalancer, error) {
	var loadBalancers []*elbv2.LoadBalancer
	req := &elbv2.DescribeLoadBalancersInput{
		PageSize: aws.Int64(20),
	}
	for {
		lbOutput, err := albClient.DescribeLoadBalancers(req)
		if err != nil {
			return nil, err
		}
		loadBalancers = append(loadBalancers, lbOutput.LoadBalancers...)
		if aws.StringValue(req.Marker) == "" {
			break
		}
		req.SetMarker(aws.StringValue(req.Marker))
	}
	return loadBalancers, nil
}

func (ep *elbV2Provider) getElbV2AndEc2Clients(region *string) ([]*elbv2.ELBV2, []*ec2.EC2) {
	albClients := make([]*elbv2.ELBV2, 0)
	ec2Clients := make([]*ec2.EC2, 0)

	albClient := elbv2.New(ep.session, aws.NewConfig().WithRegion(*region))
	albClients = append(albClients, albClient)

	ec2Client := ec2.New(ep.session, aws.NewConfig().WithRegion(*region))
	ec2Clients = append(ec2Clients, ec2Client)

	if ep.options.AssumeRoleName == "" || len(ep.options.AccountIds) < 1 {
		return albClients, ec2Clients
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
		albClients = append(albClients, elbv2.New(assumeSession))
		ec2Clients = append(ec2Clients, ec2.New(assumeSession))
	}
	return albClients, ec2Clients
}

func (ep *elbV2Provider) getLoadBalancerMetadata(lb *elbv2.LoadBalancer, albClient *elbv2.ELBV2) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "load_balancer_name", lb.LoadBalancerName)
	schema.AddMetadata(metadata, "type", lb.Type)
	schema.AddMetadata(metadata, "scheme", lb.Scheme)
	schema.AddMetadata(metadata, "dns_name", lb.DNSName)
	schema.AddMetadata(metadata, "hosted_zone_id", lb.CanonicalHostedZoneId)
	schema.AddMetadata(metadata, "vpc_id", lb.VpcId)
	schema.AddMetadata(metadata, "ip_address_type", lb.IpAddressType)
	schema.AddMetadata(metadata, "customer_owned_ipv4_pool", lb.CustomerOwnedIpv4Pool)

	if lb.LoadBalancerArn != nil {
		arn := aws.StringValue(lb.LoadBalancerArn)
		metadata["arn"] = arn

		// Extract owner ID from ARN (format: arn:aws:elasticloadbalancing:region:account-id:loadbalancer/app/name/id)
		if arnComponents := parseARN(arn); arnComponents != nil && arnComponents.AccountID != "" {
			metadata["owner_id"] = arnComponents.AccountID
		}
	}

	if lb.State != nil {
		schema.AddMetadata(metadata, "state", lb.State.Code)
		schema.AddMetadata(metadata, "state_reason", lb.State.Reason)
	}

	if lb.CreatedTime != nil {
		metadata["created_time"] = lb.CreatedTime.Format(time.RFC3339)
	}

	if len(lb.AvailabilityZones) > 0 {
		var azs []string
		var subnets []string
		for _, az := range lb.AvailabilityZones {
			if az.ZoneName != nil {
				azs = append(azs, aws.StringValue(az.ZoneName))
			}
			if az.SubnetId != nil {
				subnets = append(subnets, aws.StringValue(az.SubnetId))
			}
		}
		if len(azs) > 0 {
			metadata["availability_zones"] = strings.Join(azs, ",")
		}
		if len(subnets) > 0 {
			metadata["subnet_ids"] = strings.Join(subnets, ",")
		}
	}
	schema.AddMetadataList(metadata, "security_groups", lb.SecurityGroups)
	if lb.LoadBalancerArn != nil {
		if listeners, err := albClient.DescribeListeners(&elbv2.DescribeListenersInput{
			LoadBalancerArn: lb.LoadBalancerArn,
		}); err == nil && listeners.Listeners != nil {
			schema.AddMetadataInt(metadata, "listener_count", len(listeners.Listeners))

			var ports []string
			var protocols []string
			for _, listener := range listeners.Listeners {
				if listener.Port != nil {
					ports = append(ports, fmt.Sprintf("%d", aws.Int64Value(listener.Port)))
				}
				if listener.Protocol != nil {
					protocols = append(protocols, aws.StringValue(listener.Protocol))
				}
			}
			if len(ports) > 0 {
				metadata["listener_ports"] = strings.Join(ports, ",")
			}
			if len(protocols) > 0 {
				metadata["listener_protocols"] = strings.Join(protocols, ",")
			}
		}
	}
	if lb.LoadBalancerArn != nil {
		if tagOutput, err := albClient.DescribeTags(&elbv2.DescribeTagsInput{
			ResourceArns: []*string{lb.LoadBalancerArn},
		}); err == nil && tagOutput.TagDescriptions != nil && len(tagOutput.TagDescriptions) > 0 {
			for _, tagDesc := range tagOutput.TagDescriptions {
				if tagString := buildELBv2TagString(tagDesc.Tags); tagString != "" {
					metadata["tags"] = tagString
					break
				}
			}
		}
	}

	return metadata
}

func (ep *elbV2Provider) getTargetInstanceMetadata(instance *ec2.Instance, target *elbv2.TargetHealthDescription, tg *elbv2.TargetGroup, lb *elbv2.LoadBalancer) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "instance_id", instance.InstanceId)
	schema.AddMetadata(metadata, "instance_type", instance.InstanceType)
	schema.AddMetadata(metadata, "private_dns_name", instance.PrivateDnsName)

	if instance.State != nil {
		schema.AddMetadata(metadata, "instance_state", instance.State.Name)
	}
	if target.Target != nil && target.Target.Port != nil {
		metadata["target_port"] = fmt.Sprintf("%d", aws.Int64Value(target.Target.Port))
	}
	if target.TargetHealth != nil {
		schema.AddMetadata(metadata, "health_state", target.TargetHealth.State)
		schema.AddMetadata(metadata, "health_reason", target.TargetHealth.Reason)
		schema.AddMetadata(metadata, "health_description", target.TargetHealth.Description)
	}
	schema.AddMetadata(metadata, "target_group_name", tg.TargetGroupName)
	schema.AddMetadata(metadata, "target_type", tg.TargetType)
	schema.AddMetadata(metadata, "load_balancer_name", lb.LoadBalancerName)
	if lb.LoadBalancerArn != nil {
		arn := aws.StringValue(lb.LoadBalancerArn)

		if arnComponents := parseARN(arn); arnComponents != nil && arnComponents.AccountID != "" {
			metadata["owner_id"] = arnComponents.AccountID
		}
	}
	if len(instance.Tags) > 0 {
		if tagString := buildTagString(instance.Tags); tagString != "" {
			metadata["instance_tags"] = tagString
		}
	}
	return metadata
}

func buildELBv2TagString(tags []*elbv2.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
