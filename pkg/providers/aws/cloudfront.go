package aws

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
	var errs []error

	for _, client := range cp.getCloudfrontClients() {
		wg.Add(1)

		go func(cloudfrontClient *cloudfront.CloudFront) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("panic in cloudfront provider: %v", r))
					mu.Unlock()
				}
			}()

			if resources, err := cp.listCloudFrontResources(cloudfrontClient); err == nil {
				mu.Lock()
				list.Merge(resources)
				mu.Unlock()
			}
		}(client)
	}
	wg.Wait()
	if len(errs) > 0 && len(list.Items) == 0 {
		return nil, fmt.Errorf("cloudfront: all workers failed: %v", errs)
	}
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
			// Extract metadata for this distribution
			var metadata map[string]string
			if cp.options.ExtendedMetadata {
				metadata = cp.getDistributionMetadata(distribution, cloudFrontClient)
			}

			resource := &schema.Resource{
				Provider: "aws",
				ID:       aws.StringValue(distribution.Id),
				DNSName:  aws.StringValue(distribution.DomainName),
				Public:   true,
				Service:  cp.name(),
				Metadata: metadata,
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

func (cp *cloudfrontProvider) getDistributionMetadata(distribution *cloudfront.DistributionSummary, cloudFrontClient *cloudfront.CloudFront) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "distribution_id", distribution.Id)
	schema.AddMetadata(metadata, "status", distribution.Status)
	schema.AddMetadata(metadata, "domain_name", distribution.DomainName)
	schema.AddMetadata(metadata, "comment", distribution.Comment)
	schema.AddMetadata(metadata, "http_version", distribution.HttpVersion)
	schema.AddMetadata(metadata, "price_class", distribution.PriceClass)
	schema.AddMetadata(metadata, "web_acl_id", distribution.WebACLId)

	if distribution.ARN != nil {
		arn := aws.StringValue(distribution.ARN)
		metadata["arn"] = arn

		if arnComponents := parseARN(arn); arnComponents != nil && arnComponents.AccountID != "" {
			metadata["owner_id"] = arnComponents.AccountID
		}
	}

	if distribution.LastModifiedTime != nil {
		metadata["last_modified"] = distribution.LastModifiedTime.Format(time.RFC3339)
	}

	if distribution.Enabled != nil {
		metadata["enabled"] = fmt.Sprintf("%v", aws.BoolValue(distribution.Enabled))
	}

	if distribution.IsIPV6Enabled != nil {
		metadata["ipv6_enabled"] = fmt.Sprintf("%v", aws.BoolValue(distribution.IsIPV6Enabled))
	}

	if distribution.Aliases != nil && distribution.Aliases.Items != nil && len(distribution.Aliases.Items) > 0 {
		var aliases []string
		for _, alias := range distribution.Aliases.Items {
			if alias != nil {
				aliases = append(aliases, aws.StringValue(alias))
			}
		}
		if len(aliases) > 0 {
			metadata["aliases"] = strings.Join(aliases, ",")
		}
	}

	if distribution.Origins != nil && distribution.Origins.Items != nil && len(distribution.Origins.Items) > 0 {
		var origins []string
		for _, origin := range distribution.Origins.Items {
			if origin.DomainName != nil {
				origins = append(origins, aws.StringValue(origin.DomainName))
			}
		}
		if len(origins) > 0 {
			metadata["origins"] = strings.Join(origins, ",")
		}
		schema.AddMetadataInt(metadata, "origins_count", len(distribution.Origins.Items))
	}

	if distribution.Id != nil {
		if tagOutput, err := cloudFrontClient.ListTagsForResource(&cloudfront.ListTagsForResourceInput{
			Resource: distribution.ARN,
		}); err == nil && tagOutput.Tags != nil && tagOutput.Tags.Items != nil {
			if tagString := buildCloudFrontTagString(tagOutput.Tags.Items); tagString != "" {
				metadata["tags"] = tagString
			}
		}
	}

	return metadata
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

func buildCloudFrontTagString(tags []*cloudfront.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
