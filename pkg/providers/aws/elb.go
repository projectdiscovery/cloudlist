package aws

import (
	"context"
	"fmt"
	"sync"

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
		resource := &schema.Resource{
			Provider: "aws",
			ID:       *lb.LoadBalancerName,
			DNSName:  elbDNS,
			Public:   true,
			Service:  ep.name(),
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
