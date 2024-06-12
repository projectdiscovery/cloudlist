package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// cloudfrontProvider is a provider for AWS CloudFront API
type cloudfrontProvider struct {
	id               string
	cloudFrontClient *cloudfront.CloudFront
	session          *session.Session
}

// GetResource returns all the resources in the store for a provider.
func (cp *cloudfrontProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	resources, err := listCloudFrontResources(cp.cloudFrontClient)
	if err != nil {
		return nil, errors.Wrap(err, "could not list CloudFront resources")
	}
	return resources, nil
}

func listCloudFrontResources(cloudFrontClient *cloudfront.CloudFront) (*schema.Resources, error) {
	list := schema.NewResources()
	req := &cloudfront.ListDistributionsInput{MaxItems: aws.Int64(400)}
	for {
		distributions, err := cloudFrontClient.ListDistributions(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not list distributions")
		}

		for _, distribution := range distributions.DistributionList.Items {
			resource := &schema.Resource{
				Provider: "aws",
				ID:       aws.StringValue(distribution.Id),
				DNSName:  aws.StringValue(distribution.DomainName),
				Public:   true,
			}
			list.Append(resource)
		}
		if aws.StringValue(distributions.DistributionList.NextMarker) == "" {
			break
		}
		req.SetMarker(aws.StringValue(distributions.DistributionList.NextMarker))
	}
	return list, nil
}
