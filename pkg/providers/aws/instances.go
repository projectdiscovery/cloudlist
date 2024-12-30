package aws

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// awsInstanceProvider is an instance provider for aws API
type instanceProvider struct {
	options   ProviderOptions
	ec2Client *ec2.EC2
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetResource returns all the resources in the store for a provider.
func (i *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range i.regions.Regions {
		for _, ec2Client := range i.getEc2Clients(region.RegionName) {
			wg.Add(1)

			go func(ec2Client *ec2.EC2) {
				defer wg.Done()

				if resources, err := i.getEC2Resources(ec2Client); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(ec2Client)
		}
	}
	wg.Wait()
	return list, nil
}

func (i *instanceProvider) getEC2Resources(ec2Client *ec2.EC2) (*schema.Resources, error) {
	list := schema.NewResources()

	req := &ec2.DescribeInstancesInput{
		MaxResults: aws.Int64(1000),
	}
	for {
		resp, err := ec2Client.DescribeInstances(req)
		if err != nil {
			return nil, err
		}

		for _, reservation := range resp.Reservations {
			for _, instance := range reservation.Instances {
				ip4 := aws.StringValue(instance.PublicIpAddress)
				ip6 := aws.StringValue(instance.Ipv6Address)
				privateIp4 := aws.StringValue(instance.PrivateIpAddress)

				if privateIp4 != "" {
					list.Append(&schema.Resource{
						ID:          i.options.Id,
						Provider:    providerName,
						PrivateIpv4: privateIp4,
						Public:      false,
						Service:     i.name(),
					})
				}
				list.Append(&schema.Resource{
					ID:         i.options.Id,
					Provider:   providerName,
					PublicIPv4: ip4,
					PublicIPv6: ip6,
					Public:     true,
					Service:    i.name(),
				})
			}
		}
		if aws.StringValue(resp.NextToken) == "" {
			break
		}
		req.SetNextToken(aws.StringValue(resp.NextToken))
	}
	return list, nil
}

func (i *instanceProvider) getEc2Clients(region *string) []*ec2.EC2 {
	endpoint := fmt.Sprintf("https://ec2.%s.amazonaws.com", aws.StringValue(region))
	ec2Clients := make([]*ec2.EC2, 0)

	ec2Client := ec2.New(
		i.session,
		aws.NewConfig().WithEndpoint(endpoint),
		aws.NewConfig().WithRegion(aws.StringValue(region)),
	)
	ec2Clients = append(ec2Clients, ec2Client)

	if i.options.AssumeRoleName == "" || len(i.options.AccountIds) < 1 {
		return ec2Clients
	}

	for _, accountId := range i.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, i.options.AssumeRoleName)
		creds := stscreds.NewCredentials(i.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      region,
			Credentials: creds,
		})
		if err != nil {
			continue
		}

		ec2Clients = append(ec2Clients, ec2.New(assumeSession))
	}
	return ec2Clients
}
