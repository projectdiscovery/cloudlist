package gcp

import (
	"context"
	"fmt"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/iterator"
)

type cloudStorageProvider struct {
	id          string
	projects    []string
	assetClient *asset.Client
}

func (d *cloudStorageProvider) name() string {
	return "s3"
}

// GetResource returns all the storage resources in the store for a provider.
func (d *cloudStorageProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	for _, project := range d.projects {
		req := &assetpb.ListAssetsRequest{
			Parent:      "projects/" + project,
			AssetTypes:  []string{"storage.googleapis.com/Bucket"},
			ContentType: assetpb.ContentType_RESOURCE,
		}
		it := d.assetClient.ListAssets(ctx, req)
		for {
			asset, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to list assets for project %s: %w", project, err)
			}

			if asset.Resource == nil || asset.Resource.Data == nil {
				continue
			}
			nameField, ok := asset.Resource.Data.Fields["name"]
			if !ok || nameField == nil {
				continue
			}
			bucketName := nameField.GetStringValue()

			isPublic := false
			if asset.IamPolicy != nil {
				for _, binding := range asset.IamPolicy.Bindings {
					for _, member := range binding.Members {
						if member == "allUsers" || member == "allAuthenticatedUsers" {
							isPublic = true
							break
						}
					}
					if isPublic {
						break
					}
				}
			}

			resource := &schema.Resource{
				ID:       d.id,
				Provider: providerName,
				DNSName:  bucketName + ".storage.googleapis.com",
				Public:   isPublic,
				Service:  d.name(),
			}
			list.Append(resource)
		}
	}
	return list, nil
}
