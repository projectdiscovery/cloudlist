package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// awsInstanceProvider is an instance provider for aws API
type instanceProvider struct {
	options                ProviderOptions
	ec2Client              *ec2.EC2
	session                *session.Session
	regions                *ec2.DescribeRegionsOutput
	assumeRoleCanAccessEc2 bool
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetResource returns all the resources in the store for a provider.
func (i *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	i.assumeRoleCanAccessEc2 = true

	for _, region := range i.regions.Regions {
		for index, ec2Client := range i.getEc2Clients(region.RegionName) {
			req := &ec2.DescribeInstancesInput{
				MaxResults: aws.Int64(1000),
			}
			for {
				resp, err := ec2Client.DescribeInstances(req)

				if err != nil {
					if awsErr, ok := err.(awserr.Error); ok {
						// primary account does not have access to ec2, return error
						if awsErr.Code() == "UnauthorizedOperation" && index == 0 {
							return nil, err
						}
						// assume role account does not have access to ec2
						if awsErr.Code() == "UnauthorizedOperation" && index == 1 {
							i.assumeRoleCanAccessEc2 = false
							break
						}
					} else {
						break
					}
				}

				for _, reservation := range resp.Reservations {
					for _, instance := range reservation.Instances {
						ip4 := aws.StringValue(instance.PublicIpAddress)
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
		}
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

	if i.options.AssumeRoleName == "" || len(i.options.AccountIds) < 1 || !i.assumeRoleCanAccessEc2 {
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
			break
		}

		ec2Clients = append(ec2Clients, ec2.New(assumeSession))

	}

	return ec2Clients
}
