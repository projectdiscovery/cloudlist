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

// GetResource returns all the storage resources in the store for a provider.
func (d *cloudStorageProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		bucketsService := d.storage.Buckets.List(project)
		err := bucketsService.Pages(context.Background(), func(bal *storage.Buckets) error {
			for _, bucket := range bal.Items {
				resource := &schema.Resource{
					ID:       d.id,
					Provider: providerName,
					DNSName:  fmt.Sprintf("%s.storage.googleapis.com", bucket.Name),
				}
				bucketIAMPolicy, err := d.storage.Buckets.GetIamPolicy(bucket.Name).Do()
				if err == nil {
					for _, binding := range bucketIAMPolicy.Bindings {
						if binding.Role == "roles/storage.objectViewer" {
							for _, member := range binding.Members {
								if member == "allUsers" || member == "allAuthenticatedUsers" {
									resource.Public = true
									break
								}
							}
						}
					}
				}
				list.Append(resource)
			}
			return nil
		})
		if err != nil {
			continue
		}
	}
	return list, nil
}
