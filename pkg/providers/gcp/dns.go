package gcp

import (
	"context"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/iterator"
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
		req := &assetpb.ListAssetsRequest{
			Parent:      "projects/" + project,
			AssetTypes:  []string{"dns.googleapis.com/ManagedZone"},
			ContentType: assetpb.ContentType_RESOURCE,
		}
		it := d.assetClient.ListAssets(ctx, req)
		for {
			zoneAsset, err := it.Next()
			if err == iterator.Done {
				break
			}

			isPublic := true
			if zoneAsset.Resource != nil && zoneAsset.Resource.Data != nil {
				fields := zoneAsset.Resource.Data.Fields
				if visibilityField, ok := fields["visibility"]; ok {
					isPublic = visibilityField.GetStringValue() == "public"
				}
			}

			recordsReq := &assetpb.ListAssetsRequest{
				Parent:      "projects/" + project,
				AssetTypes:  []string{"dns.googleapis.com/ResourceRecordSet"},
				ContentType: assetpb.ContentType_RESOURCE,
			}

			recordsIt := d.assetClient.ListAssets(ctx, recordsReq)
			for {
				recordAsset, err := recordsIt.Next()
				if err == iterator.Done {
					break
				}

				if recordAsset.Resource != nil && recordAsset.Resource.Data != nil {
					fields := recordAsset.Resource.Data.Fields
					recordName := fields["name"].GetStringValue()
					recordType := fields["type"].GetStringValue()

					if recordType != "A" && recordType != "CNAME" && recordType != "AAAA" {
						continue
					}

					if rrdatasField, ok := fields["rrdatas"]; ok {
						rrdatas := rrdatasField.GetListValue().Values
						for _, rdata := range rrdatas {
							data := rdata.GetStringValue()

							resource := &schema.Resource{
								DNSName:  recordName,
								Public:   isPublic,
								ID:       d.id,
								Provider: providerName,
								Service:  d.name(),
							}

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
	}
	return list, nil
}
