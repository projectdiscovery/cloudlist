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
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// s3Provider is a provider for aws S3 API
type s3Provider struct {
	options ProviderOptions
	s3      *s3.S3
	session *session.Session
}

func (s *s3Provider) name() string {
	return "s3"
}

// GetResource returns all the resources in the store for a provider.
func (s *s3Provider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, s3Client := range s.getS3Clients() {
		wg.Add(1)

		go func(s3Client *wrappedS3Client) {
			defer wg.Done()

			if resources, err := s.getS3Resources(s3Client); err == nil {
				mu.Lock()
				list.Merge(resources)
				mu.Unlock()
			}
		}(s3Client)
	}
	wg.Wait()
	return list, nil
}

func (s *s3Provider) getS3Resources(s3Client *wrappedS3Client) (*schema.Resources, error) {
	list := schema.NewResources()
	req := &s3.ListBucketsInput{}
	listBucketsOutput, err := s3Client.s3Client.ListBuckets(req)
	if err != nil {
		return nil, errors.Wrap(err, "could not list s3 buckets")
	}

	for _, bucket := range listBucketsOutput.Buckets {
		// Extract metadata for this bucket
		var metadata map[string]string
		if s.options.ExtendedMetadata {
			metadata = s.getBucketMetadata(bucket, listBucketsOutput.Owner, s3Client)
		}

		list.Append(&schema.Resource{
			ID:       s.options.Id,
			Public:   true,
			DNSName:  fmt.Sprintf("%s.s3.amazonaws.com", aws.StringValue(bucket.Name)),
			Provider: providerName,
			Service:  s.name(),
			Metadata: metadata,
		})
	}
	return list, nil
}

func (s *s3Provider) getBucketMetadata(bucket *s3.Bucket, owner *s3.Owner, s3Client *wrappedS3Client) map[string]string {
	metadata := make(map[string]string)

	// Basic bucket information
	schema.AddMetadata(metadata, "bucket_name", bucket.Name)
	if bucket.CreationDate != nil {
		metadata["creation_date"] = bucket.CreationDate.Format(time.RFC3339)
	}

	// Owner information
	if owner != nil {
		schema.AddMetadata(metadata, "owner_id", owner.ID)
		schema.AddMetadata(metadata, "owner_display_name", owner.DisplayName)
	}

	bucketName := aws.StringValue(bucket.Name)
	if bucketName == "" {
		return metadata
	}

	// Get bucket location (region)
	var bucketRegion string
	if location, err := s3Client.s3Client.GetBucketLocation(&s3.GetBucketLocationInput{
		Bucket: bucket.Name,
	}); err == nil {
		region := aws.StringValue(location.LocationConstraint)
		if region == "" {
			region = "us-east-1"
		}
		bucketRegion = region
		metadata["region"] = region
	} else {
		return metadata // If we cannot get region, return with current all we have
	}

	// Create a region-specific client if needed
	regionClient := s.getS3Client(bucketRegion, s3Client.roleARN)
	if regionClient == nil {
		regionClient = s3Client.s3Client
	}

	// Get bucket tags
	if tagOutput, err := regionClient.GetBucketTagging(&s3.GetBucketTaggingInput{
		Bucket: bucket.Name,
	}); err == nil && tagOutput.TagSet != nil {
		if tagString := buildS3TagString(tagOutput.TagSet); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	// Get public access block configuration
	if publicAccessBlock, err := regionClient.GetPublicAccessBlock(&s3.GetPublicAccessBlockInput{
		Bucket: bucket.Name,
	}); err == nil && publicAccessBlock.PublicAccessBlockConfiguration != nil {
		config := publicAccessBlock.PublicAccessBlockConfiguration

		// A bucket is considered public if ALL public access blocks are disabled
		isPublic := !aws.BoolValue(config.BlockPublicAcls) &&
			!aws.BoolValue(config.BlockPublicPolicy) &&
			!aws.BoolValue(config.IgnorePublicAcls) &&
			!aws.BoolValue(config.RestrictPublicBuckets)
		if isPublic {
			metadata["public_access"] = "enabled"
		}
	}
	return metadata
}

type wrappedS3Client struct {
	s3Client *s3.S3
	roleARN  string
}

func (s *s3Provider) getS3Clients() []*wrappedS3Client {
	s3Clients := make([]*wrappedS3Client, 0)
	s3Clients = append(s3Clients, &wrappedS3Client{s3Client: s.s3})

	if s.options.AssumeRoleName == "" || len(s.options.AccountIds) < 1 {
		return s3Clients
	}

	for _, accountId := range s.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, s.options.AssumeRoleName)
		s3Client := s.getS3Client("us-east-1", roleARN)
		if s3Client == nil {
			continue
		}
		s3Clients = append(s3Clients, &wrappedS3Client{s3Client: s3Client, roleARN: roleARN})
	}
	return s3Clients
}

func (s *s3Provider) getS3Client(region, roleARN string) *s3.S3 {
	// If no roleARN provided, return a region-specific client using existing session
	if roleARN == "" {
		if region == "us-east-1" {
			return s.s3
		}
		return s3.New(s.session, &aws.Config{
			Region: aws.String(region),
		})
	}

	// For role-based access, create a new session with assumed role credentials
	creds := stscreds.NewCredentials(s.session, roleARN)
	assumeSession, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: creds,
	})
	if err != nil {
		return nil
	}
	return s3.New(assumeSession)
}

func buildS3TagString(tags []*s3.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
