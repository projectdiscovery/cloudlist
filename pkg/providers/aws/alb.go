package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
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

	for _, region := range ep.regions.Regions {
		regionName := *region.RegionName
		albClient := elbv2.New(ep.session, aws.NewConfig().WithRegion(regionName))
		ec2Client := ec2.New(ep.session, aws.NewConfig().WithRegion(regionName))
		if resources, err := ep.listELBV2Resources(albClient, ec2Client); err == nil {
			list.Merge(resources)
		}
	}
	return list, nil
}

func (ep *elbV2Provider) listELBV2Resources(albClient *elbv2.ELBV2, ec2Client *ec2.EC2) (*schema.Resources, error) {
	list := schema.NewResources()

	loadBalancers, err := ep.getLoadBalancers(albClient)
	if err != nil {
		return nil, errors.Wrap(err, "could not list load balancers")
	}

	for _, lb := range loadBalancers {
		albDNS := *lb.DNSName
		resource := &schema.Resource{
			Provider: "aws",
			ID:       *lb.LoadBalancerName,
			DNSName:  albDNS,
			Public:   true,
			Service:  ep.name(),
		}
		list.Append(resource)
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
							resource := &schema.Resource{
								Provider:    "aws",
								ID:          instanceID,
								PrivateIpv4: *instance.PrivateIpAddress,
								Public:      false,
								Service:     ep.name(),
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
			return nil, errors.Wrap(err, "could not describe load balancers")
		}
		loadBalancers = append(loadBalancers, lbOutput.LoadBalancers...)
		if aws.StringValue(req.Marker) == "" {
			break
		}
		req.SetMarker(aws.StringValue(req.Marker))
	}
	return loadBalancers, nil
}
