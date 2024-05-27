package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// lightsailProvider is an instance provider for AWS Lightsail API
type lightsailProvider struct {
	id       string
	lsClient *lightsail.Lightsail
	session  *session.Session
	regions  []*lightsail.Region
}

// GetResource returns all the resources in the store for a provider.
func (d *lightsailProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, region := range d.regions {
		endpointBuilder := &strings.Builder{}
		endpointBuilder.WriteString("https://lightsail.")
		endpointBuilder.WriteString(aws.StringValue(region.Name))
		endpointBuilder.WriteString(".amazonaws.com")

		lsClient := lightsail.New(
			d.session,
			aws.NewConfig().WithEndpoint(endpointBuilder.String()),
			aws.NewConfig().WithRegion(aws.StringValue(region.Name)),
		)
		req := &lightsail.GetInstancesInput{}
		for {
			resp, err := lsClient.GetInstances(req)
			if err != nil {
				return nil, errors.Wrap(err, "could not describe instances")
			}

			for _, instance := range resp.Instances {
				privateIPv4 := aws.StringValue(instance.PrivateIpAddress)
				publicIPv4 := aws.StringValue(instance.PublicIpAddress)
				resource := &schema.Resource{
					ID:          d.id,
					Provider:    providerName,
					PrivateIpv4: privateIPv4,
					PublicIPv4:  publicIPv4,
					Public:      publicIPv4 != "",
				}
				list.Append(resource)
			}
			if aws.StringValue(resp.NextPageToken) == "" {
				break
			}
			req.PageToken = resp.NextPageToken
		}
	}
	return list, nil
}
