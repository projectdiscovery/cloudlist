package gcp

import (
	"context"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// gkeProvider is a provider for GCP GKE API
type gkeProvider struct {
	id          string
	assetClient *asset.Client
	projects    []string
}

func (d *gkeProvider) name() string {
	return "gke"
}

// GetResource returns all the GKE resources in the store for a provider.
func (d *gkeProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		req := &assetpb.ListAssetsRequest{
			Parent:      "projects/" + project,
			AssetTypes:  []string{"container.googleapis.com/Cluster"},
			ContentType: assetpb.ContentType_RESOURCE,
		}
		it := d.assetClient.ListAssets(ctx, req)
		for {
			asset, err := it.Next()
			if err != nil {
				break
			}
			clusterName := asset.Resource.Data.Fields["name"].GetStringValue()

			// Default to private (GKE clusters are typically private)
			isPublic := false
			if asset.IamPolicy != nil {
				for _, binding := range asset.IamPolicy.Bindings {
					if binding.Role == "roles/container.clusterViewer" || binding.Role == "roles/container.admin" {
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
			}

			resource := &schema.Resource{
				ID:       d.id,
				Provider: providerName,
				DNSName:  clusterName + ".gke.io",
				Public:   isPublic,
				Service:  d.name(),
			}
			list.Append(resource)
		}
	}
	return list, nil
}
