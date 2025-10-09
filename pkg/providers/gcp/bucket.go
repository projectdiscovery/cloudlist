package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/storage/v1"
)

type cloudStorageProvider struct {
	id               string
	storage          *storage.Service
	projects         []string
	extendedMetadata bool
}

func (d *cloudStorageProvider) name() string {
	return "s3"
}

// GetResource returns all the storage resources in the store for a provider.
func (d *cloudStorageProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	buckets, err := d.getBuckets()
	if err != nil {
		return nil, fmt.Errorf("could not get buckets: %s", err)
	}
	for _, bucket := range buckets {
		// Extract metadata for this bucket
		var metadata map[string]string
		if d.extendedMetadata {
			metadata = d.getBucketMetadata(bucket)
		}

		resource := &schema.Resource{
			ID:       d.id,
			Provider: providerName,
			DNSName:  fmt.Sprintf("%s.storage.googleapis.com", bucket.Name),
			Public:   d.isBucketPublic(bucket.Name),
			Service:  d.name(),
			Metadata: metadata,
		}
		list.Append(resource)
	}
	return list, nil
}

func (d *cloudStorageProvider) getBuckets() ([]*storage.Bucket, error) {
	var buckets []*storage.Bucket
	for _, project := range d.projects {
		bucketsService := d.storage.Buckets.List(project)
		_ = bucketsService.Pages(context.Background(), func(bal *storage.Buckets) error {
			buckets = append(buckets, bal.Items...)
			return nil
		})
	}
	return buckets, nil
}

func (d *cloudStorageProvider) isBucketPublic(bucketName string) bool {
	bucketIAMPolicy, err := d.storage.Buckets.GetIamPolicy(bucketName).Do()
	if err == nil {
		for _, binding := range bucketIAMPolicy.Bindings {
			if binding.Role == "roles/storage.objectViewer" {
				for _, member := range binding.Members {
					if member == "allUsers" || member == "allAuthenticatedUsers" {
						return true
					}
				}
			}
		}
	}
	return false
}

func (d *cloudStorageProvider) getBucketMetadata(bucket *storage.Bucket) map[string]string {
	metadata := make(map[string]string)

	metadata["bucket_name"] = bucket.Name
	if bucket.TimeCreated != "" {
		if createdTime, err := time.Parse(time.RFC3339, bucket.TimeCreated); err == nil {
			metadata["creation_date"] = createdTime.Format(time.RFC3339)
		}
	}
	if bucket.Updated != "" {
		if updatedTime, err := time.Parse(time.RFC3339, bucket.Updated); err == nil {
			metadata["last_modified"] = updatedTime.Format(time.RFC3339)
		}
	}

	metadata["project_id"] = fmt.Sprintf("%d", bucket.ProjectNumber)
	metadata["owner_id"] = fmt.Sprintf("%d", bucket.ProjectNumber) // In GCP, project number serves as owner identifier

	schema.AddMetadata(metadata, "location", &bucket.Location)
	schema.AddMetadata(metadata, "location_type", &bucket.LocationType)
	schema.AddMetadata(metadata, "storage_class", &bucket.StorageClass)

	if len(bucket.Labels) > 0 {
		var labelPairs []string
		for key, value := range bucket.Labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", key, value))
		}
		metadata["labels"] = strings.Join(labelPairs, ",")
	}
	if bucket.Versioning != nil && bucket.Versioning.Enabled {
		metadata["versioning"] = "enabled"
	}
	if bucket.Lifecycle != nil && len(bucket.Lifecycle.Rule) > 0 {
		metadata["lifecycle_rules_count"] = fmt.Sprintf("%d", len(bucket.Lifecycle.Rule))

		// Extract lifecycle rule types
		var ruleTypes []string
		for _, rule := range bucket.Lifecycle.Rule {
			if rule.Action != nil && rule.Action.Type != "" {
				ruleTypes = append(ruleTypes, rule.Action.Type)
			}
		}
		if len(ruleTypes) > 0 {
			metadata["lifecycle_rule_types"] = strings.Join(ruleTypes, ",")
		}
	}
	if bucket.Encryption != nil && bucket.Encryption.DefaultKmsKeyName != "" {
		metadata["encryption_key"] = bucket.Encryption.DefaultKmsKeyName
		metadata["encryption"] = "enabled"
	}
	if bucket.Website != nil {
		if bucket.Website.MainPageSuffix != "" {
			metadata["website_main_page"] = bucket.Website.MainPageSuffix
		}
		if bucket.Website.NotFoundPage != "" {
			metadata["website_404_page"] = bucket.Website.NotFoundPage
		}
	}

	if len(bucket.Cors) > 0 {
		metadata["cors_enabled"] = "true"
		metadata["cors_rules_count"] = fmt.Sprintf("%d", len(bucket.Cors))
	}

	if bucket.IamConfiguration != nil && bucket.IamConfiguration.UniformBucketLevelAccess != nil {
		if bucket.IamConfiguration.UniformBucketLevelAccess.Enabled {
			metadata["uniform_bucket_level_access"] = "enabled"
		}
		if bucket.IamConfiguration.UniformBucketLevelAccess.LockedTime != "" {
			metadata["uniform_access_locked_time"] = bucket.IamConfiguration.UniformBucketLevelAccess.LockedTime
		}
	}

	if bucket.IamConfiguration != nil && bucket.IamConfiguration.PublicAccessPrevention != "" {
		metadata["public_access_prevention"] = bucket.IamConfiguration.PublicAccessPrevention
	}
	metadata["metageneration"] = fmt.Sprintf("%d", bucket.Metageneration)
	schema.AddMetadata(metadata, "etag", &bucket.Etag)
	schema.AddMetadata(metadata, "self_link", &bucket.SelfLink)
	if bucket.Billing != nil && bucket.Billing.RequesterPays {
		metadata["requester_pays"] = "true"
	}
	return metadata
}
