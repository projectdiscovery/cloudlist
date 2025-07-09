package gcp

import (
	"context"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// cloudDNSProvider is a provider for GCP Cloud DNS API
type cloudDNSProvider struct {
	id          string
	assetClient *asset.Client
	projects    []string
}

func (d *cloudDNSProvider) name() string {
	return "dns"
}

// GetResource returns all the DNS resources in the store for a provider.
func (d *cloudDNSProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		// Get resource record sets for this zone
		recordsReq := &assetpb.ListAssetsRequest{
			Parent:      "projects/" + project,
			AssetTypes:  []string{"dns.googleapis.com/ResourceRecordSet"},
			ContentType: assetpb.ContentType_RESOURCE,
		}
		recordsIt := d.assetClient.ListAssets(ctx, recordsReq)
		for {
			recordAsset, err := recordsIt.Next()
			if err != nil {
				break
			}

			// Parse record data
			if recordAsset.Resource != nil && recordAsset.Resource.Data != nil {
				fields := recordAsset.Resource.Data.Fields
				recordName := fields["name"].GetStringValue()
				recordType := fields["type"].GetStringValue()

				// Only process A, CNAME, and AAAA records
				if recordType != "A" && recordType != "CNAME" && recordType != "AAAA" {
					continue
				}

				// Get record data
				if rrdatasField, ok := fields["rrdatas"]; ok {
					rrdatas := rrdatasField.GetListValue().Values
					for _, rdata := range rrdatas {
						data := rdata.GetStringValue()

						resource := &schema.Resource{
							DNSName:  recordName,
							Public:   true, // DNS records are typically public
							ID:       d.id,
							Provider: providerName,
							Service:  d.name(),
						}

						// Set IP addresses based on record type
						switch recordType {
						case "A":
							resource.PublicIPv4 = data
						case "AAAA":
							resource.PublicIPv6 = data
						}

						list.Append(resource)
					}
				}
			}
		}
	}
	return list, nil
}
