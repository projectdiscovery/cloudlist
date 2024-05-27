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

// GetResource returns all the resources in the store for a provider.
func (ep *elbProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, region := range ep.regions.Regions {
		regionName := *region.RegionName
		sess, err := session.NewSession(&aws.Config{
			Endpoint: aws.String("http://localhost:4566"),
			Region:   aws.String(regionName)},
		)
		if err != nil {
			return nil, errors.Wrapf(err, "could not create session for region %s", regionName)
		}
		elbClient := elb.New(sess)
		ec2Client := ec2.New(sess)
		err = listELBResources(elbClient, ec2Client, list)
		if err != nil {
			return nil, errors.Wrapf(err, "could not list ELB resources for region %s", regionName)
		}
	}
	return list, nil
}

func listELBResources(elbClient *elb.ELB, ec2Client *ec2.EC2, list *schema.Resources) error {
	lbOutput, err := elbClient.DescribeLoadBalancers(nil)
	if err != nil {
		return errors.Wrap(err, "could not describe load balancers")
	}

	for _, lb := range lbOutput.LoadBalancerDescriptions {
		elbDNS := *lb.DNSName
		resource := &schema.Resource{
			Provider: "aws",
			ID:       *lb.LoadBalancerName,
			DNSName:  elbDNS,
			Public:   true,
		}
		list.Append(resource)
		// Describe Instances for the Load Balancer
		for _, instance := range lb.Instances {
			instanceID := *instance.InstanceId
			instanceOutput, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
				InstanceIds: []*string{&instanceID},
			})
			if err != nil {
				return errors.Wrapf(err, "could not describe instance %s", instanceID)
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
						}
						list.Append(resource)
					}
				}
			}
		}
	}
	return nil
}
