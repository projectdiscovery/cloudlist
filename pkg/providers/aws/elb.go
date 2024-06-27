package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// elbProvider is a provider for AWS Elastic Load Balancing (ELB) resources
type elbProvider struct {
	id        string
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

	for _, region := range ep.regions.Regions {
		regionName := *region.RegionName
		elbClient := elb.New(ep.session, aws.NewConfig().WithRegion(regionName))
		ec2Client := ec2.New(ep.session, aws.NewConfig().WithRegion(regionName))
		if resources, err := ep.listELBResources(elbClient, ec2Client); err == nil {
			list.Merge(resources)
		}
	}
	return list, nil
}

func (ep *elbProvider) listELBResources(elbClient *elb.ELB, ec2Client *ec2.EC2) (*schema.Resources, error) {
	list := schema.NewResources()
	req := &elb.DescribeLoadBalancersInput{}
	for {
		lbOutput, err := elbClient.DescribeLoadBalancers(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not describe load balancers")
		}

		for _, lb := range lbOutput.LoadBalancerDescriptions {
			elbDNS := *lb.DNSName
			resource := &schema.Resource{
				Provider: "aws",
				ID:       *lb.LoadBalancerName,
				DNSName:  elbDNS,
				Public:   true,
				Service:  ep.name(),
			}
			list.Append(resource)
			// Describe Instances for the Load Balancer
			for _, instance := range lb.Instances {
				instanceID := *instance.InstanceId
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
		if aws.StringValue(lbOutput.NextMarker) == "" {
			break
		}
		req.SetMarker(aws.StringValue(lbOutput.NextMarker))
	}
	return list, nil
}
