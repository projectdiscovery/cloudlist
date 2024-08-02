package aws

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// cloudfrontProvider is a provider for AWS CloudFront API
type cloudfrontProvider struct {
	options          ProviderOptions
	cloudFrontClient *cloudfront.CloudFront
	session          *session.Session
}

func (cp *cloudfrontProvider) name() string {
	return "cloudfront"
}

// GetResource returns all the resources in the store for a provider.
func (cp *cloudfrontProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, client := range cp.getCloudfrontClients() {
		wg.Add(1)

		go func(cloudfrontClient *cloudfront.CloudFront) {
			defer wg.Done()

			if resources, err := cp.listCloudFrontResources(cloudfrontClient); err == nil {
				mu.Lock()
				list.Merge(resources)
				mu.Unlock()
			}
		}(client)
	}
	wg.Wait()
	return list, nil
}

func (cp *cloudfrontProvider) listCloudFrontResources(cloudFrontClient *cloudfront.CloudFront) (*schema.Resources, error) {
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
				Service:  cp.name(),
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

func (cp *cloudfrontProvider) getCloudfrontClients() []*cloudfront.CloudFront {
	cloudfrontClients := make([]*cloudfront.CloudFront, 0)
	cloudfrontClients = append(cloudfrontClients, cp.cloudFrontClient)

	if cp.options.AssumeRoleName == "" || len(cp.options.AccountIds) < 1 {
		return cloudfrontClients
	}

	for _, accountId := range cp.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, cp.options.AssumeRoleName)
		creds := stscreds.NewCredentials(cp.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      aws.String("us-east-1"),
			Credentials: creds,
		})
		if err != nil {
			continue
		}

		cloudfrontClients = append(cloudfrontClients, cloudfront.New(assumeSession))
	}
	return cloudfrontClients
}
