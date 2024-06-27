package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// awsInstanceProvider is an instance provider for aws API
type instanceProvider struct {
	id        string
	ec2Client *ec2.EC2
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetResource returns all the resources in the store for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, region := range d.regions.Regions {
		req := &ec2.DescribeInstancesInput{
			MaxResults: aws.Int64(1000),
		}
		endpointBuilder := &strings.Builder{}
		endpointBuilder.WriteString("https://ec2.")
		endpointBuilder.WriteString(aws.StringValue(region.RegionName))
		endpointBuilder.WriteString(".amazonaws.com")

		ec2Client := ec2.New(
			d.session,
			aws.NewConfig().WithEndpoint(endpointBuilder.String()),
			aws.NewConfig().WithRegion(aws.StringValue(region.RegionName)),
		)
		for {
			resp, err := ec2Client.DescribeInstances(req)
			if err != nil {
				return nil, errors.Wrap(err, "could not describe instances")
			}
			for _, reservation := range resp.Reservations {
				for _, instance := range reservation.Instances {
					ip4 := aws.StringValue(instance.PublicIpAddress)
					privateIp4 := aws.StringValue(instance.PrivateIpAddress)

					if privateIp4 != "" {
						list.Append(&schema.Resource{
							ID:          d.id,
							Provider:    providerName,
							PrivateIpv4: privateIp4,
							Public:      false,
							Service:    d.name(),
						})
					}
					list.Append(&schema.Resource{
						ID:         d.id,
						Provider:   providerName,
						PublicIPv4: ip4,
						Public:     true,
						Service:   d.name(),
					})
				}
			}
			if aws.StringValue(resp.NextToken) == "" {
				break
			}
			req.SetNextToken(aws.StringValue(resp.NextToken))
		}
	}
	return list, nil
}
