package gcp

import (
	"context"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

type cloudVMProvider struct {
	id          string
	assetClient *asset.Client
	projects    []string
}

func (d *cloudVMProvider) name() string {
	return "vms"
}

// GetResource returns all the VM resources in the store for a provider.
func (d *cloudVMProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		req := &assetpb.ListAssetsRequest{
			Parent:      "projects/" + project,
			AssetTypes:  []string{"compute.googleapis.com/Instance"},
			ContentType: assetpb.ContentType_RESOURCE,
		}
		it := d.assetClient.ListAssets(ctx, req)
		for {
			asset, err := it.Next()
			if err != nil {
				break
			}
			instanceName := asset.Resource.Data.Fields["name"].GetStringValue()

			// Default to private
			isPublic := false
			var publicIPv4, publicIPv6 string

			// Check network interfaces for public IPs
			if networkInterfacesField, ok := asset.Resource.Data.Fields["networkInterfaces"]; ok {
				networkInterfaces := networkInterfacesField.GetListValue().Values
				for _, nic := range networkInterfaces {
					nicFields := nic.GetStructValue().Fields

					// Check access configs for public IPs
					if accessConfigsField, ok := nicFields["accessConfigs"]; ok {
						accessConfigs := accessConfigsField.GetListValue().Values
						for _, config := range accessConfigs {
							configFields := config.GetStructValue().Fields

							// Check for NAT IP (public IPv4)
							if natIPField, ok := configFields["natIP"]; ok {
								natIP := natIPField.GetStringValue()
								if natIP != "" {
									publicIPv4 = natIP
									isPublic = true
								}
							}

							// Check for external IPv6
							if externalIPv6Field, ok := configFields["externalIpv6"]; ok {
								externalIPv6 := externalIPv6Field.GetStringValue()
								if externalIPv6 != "" {
									publicIPv6 = externalIPv6
									isPublic = true
								}
							}
						}
					}
				}
			}

			resource := &schema.Resource{
				ID:         d.id,
				Provider:   providerName,
				DNSName:    instanceName + ".compute.googleapis.com",
				Public:     isPublic,
				PublicIPv4: publicIPv4,
				PublicIPv6: publicIPv6,
				Service:    d.name(),
			}
			list.Append(resource)
		}
	}
	return list, nil
}
