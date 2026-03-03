package cloudflare

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

type apiClient interface {
	ListZones(ctx context.Context, opts ...string) ([]cloudflare.Zone, error)
	ListDNSRecords(ctx context.Context, zoneID *cloudflare.ResourceContainer, params cloudflare.ListDNSRecordsParams) ([]cloudflare.DNSRecord, *cloudflare.ResultInfo, error)
}

type dnsProvider struct {
	id               string
	client           apiClient
	extendedMetadata bool
}

func (d *dnsProvider) name() string {
	return "dns"
}

// GetResource returns all the resources in the store for a provider.
func (d *dnsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	zones, err := d.client.ListZones(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not list zones")
	}
	for _, zone := range zones {
		// Fetch all records for a zone
		recs, _, err := d.client.ListDNSRecords(ctx, cloudflare.ZoneIdentifier(zone.ID), cloudflare.ListDNSRecordsParams{})
		if err != nil {
			return list, errors.Wrap(err, "could not list zones")
		}
		for _, record := range recs {
			if record.Type != "A" && record.Type != "CNAME" && record.Type != "AAAA" {
				continue
			}

			// Extract metadata for this record
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = d.getDNSRecordMetadata(&record, &zone)
			}

			// Create a single resource with both DNS name and IP
			// The schema layer will handle keeping them together for DNS service
			resource := &schema.Resource{
				Public:   true,
				Provider: providerName,
				DNSName:  record.Name,
				ID:       d.id,
				Service:  d.name(),
				Metadata: metadata,
			}

			// Add IP information based on record type
			// For A and AAAA records, include the IP in the same resource
			if record.Type == "A" && record.Content != "" {
				resource.PublicIPv4 = record.Content
			} else if record.Type == "AAAA" && record.Content != "" {
				resource.PublicIPv6 = record.Content
			}
			// For CNAME records, only DNS name is included

			// Use the standard Append method which now has special handling for DNS service
			list.Append(resource)
		}
	}
	return list, nil
}

// getDNSRecordMetadata extracts metadata for a DNS record
func (d *dnsProvider) getDNSRecordMetadata(record *cloudflare.DNSRecord, zone *cloudflare.Zone) map[string]string {
	metadata := make(map[string]string)

	// Zone information
	metadata["zone_id"] = zone.ID
	metadata["zone_name"] = zone.Name
	if zone.Account.ID != "" {
		metadata["account_id"] = zone.Account.ID
	}
	if zone.Account.Name != "" {
		metadata["account_name"] = zone.Account.Name
	}
	metadata["record_id"] = record.ID
	metadata["record_type"] = record.Type
	metadata["record_name"] = record.Name
	metadata["record_content"] = record.Content

	if record.TTL > 0 {
		metadata["ttl"] = fmt.Sprintf("%d", record.TTL)
	}
	if record.Proxied != nil {
		metadata["proxied"] = fmt.Sprintf("%v", *record.Proxied)
	}
	metadata["proxiable"] = fmt.Sprintf("%v", record.Proxiable)

	if record.Locked {
		metadata["locked"] = "true"
	}

	if record.Priority != nil {
		metadata["priority"] = fmt.Sprintf("%d", *record.Priority)
	}
	if record.Comment != "" {
		metadata["comment"] = record.Comment
	}
	if len(record.Tags) > 0 {
		metadata["tags"] = strings.Join(record.Tags, ",")
	}
	if !record.CreatedOn.IsZero() {
		metadata["created_on"] = record.CreatedOn.Format(time.RFC3339)
	}
	if !record.ModifiedOn.IsZero() {
		metadata["modified_on"] = record.ModifiedOn.Format(time.RFC3339)
	}
	metadata["zone_status"] = zone.Status
	if zone.Plan.Name != "" {
		metadata["zone_plan"] = zone.Plan.Name
	}
	metadata["zone_type"] = zone.Type

	if record.Meta != nil {
		if metaMap, ok := record.Meta.(map[string]interface{}); ok {
			for k, v := range metaMap {
				metadata[fmt.Sprintf("meta_%s", k)] = fmt.Sprintf("%v", v)
			}
		}
	}
	if record.Data != nil {
		if dataMap, ok := record.Data.(map[string]interface{}); ok {
			for k, v := range dataMap {
				metadata[fmt.Sprintf("data_%s", k)] = fmt.Sprintf("%v", v)
			}
		}
	}
	return metadata
}
