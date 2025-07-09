package gcp

import (
	"context"
	"strings"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

type cloudFunctionsProvider struct {
	id          string
	assetClient *asset.Client
	projects    []string
}

func (d *cloudFunctionsProvider) name() string {
	return "cloud-function"
}

// GetResource returns all the Cloud Function resources in the store for a provider.
func (d *cloudFunctionsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	for _, project := range d.projects {
		req := &assetpb.ListAssetsRequest{
			Parent:      "projects/" + project,
			AssetTypes:  []string{"cloudfunctions.googleapis.com/CloudFunction"},
			ContentType: assetpb.ContentType_RESOURCE,
		}
		it := d.assetClient.ListAssets(ctx, req)
		for {
			asset, err := it.Next()
			if err != nil {
				break
			}
			functionName := asset.Resource.Data.Fields["name"].GetStringValue()
			var dnsName string
			if httpsTriggerField, ok := asset.Resource.Data.Fields["httpsTrigger"]; ok {
				if httpsTrigger := httpsTriggerField.GetStructValue(); httpsTrigger != nil {
					if urlField, ok := httpsTrigger.Fields["url"]; ok {
						if url := urlField.GetStringValue(); url != "" {
							dnsName = strings.TrimPrefix(url, "https://")
						}
					}
				}
			}
			if dnsName == "" {
				dnsName = functionName + ".cloudfunctions.net"
			}

			isPublic := false
			if asset.IamPolicy != nil {
				for _, binding := range asset.IamPolicy.Bindings {
					if binding.Role == "roles/cloudfunctions.invoker" {
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
				DNSName:  dnsName,
				Public:   isPublic,
				Service:  d.name(),
			}
			list.Append(resource)
		}
	}
	return list, nil
}
