package gcp

import (
	"context"
	"fmt"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/storage/v1"
)

type cloudStorageProvider struct {
	id       string
	storage  *storage.Service
	projects []string
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
		resource := &schema.Resource{
			ID:       d.id,
			Provider: providerName,
			DNSName:  fmt.Sprintf("%s.storage.googleapis.com", bucket.Name),
			Public:   d.isBucketPublic(bucket.Name),
			Service:  d.name(),
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
