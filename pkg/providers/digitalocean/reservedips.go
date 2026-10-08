package digitalocean

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// reservedIPProvider is a reserved IP provider for digitalocean API
type reservedIPProvider struct {
	id               string
	client           *godo.Client
	extendedMetadata bool
}

func (d *reservedIPProvider) name() string {
	return "reservedip"
}

// GetResource returns all the reserved IPv4 and IPv6 addresses, including
// unassigned ones, which the droplet listing never returns.
func (d *reservedIPProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	opt := &godo.ListOptions{PerPage: 200}
	for {
		ips, resp, err := d.client.ReservedIPs.List(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, ip := range ips {
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = getReservedIPMetadata(ip.Region, ip.Droplet)
				schema.AddMetadata(metadata, "project_id", &ip.ProjectID)
				locked := fmt.Sprintf("%v", ip.Locked)
				schema.AddMetadata(metadata, "locked", &locked)
			}

			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         d.id,
				PublicIPv4: ip.IP,
				Public:     true,
				Service:    d.name(),
				Metadata:   metadata,
			})
		}
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}
		opt.Page = page + 1
	}

	opt = &godo.ListOptions{PerPage: 200}
	for {
		ips, resp, err := d.client.ReservedIPV6s.List(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, ip := range ips {
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = getReservedIPMetadata(&godo.Region{Slug: ip.RegionSlug}, ip.Droplet)
				if !ip.ReservedAt.IsZero() {
					reservedAt := ip.ReservedAt.Format(time.RFC3339)
					schema.AddMetadata(metadata, "reserved_at", &reservedAt)
				}
			}

			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         d.id,
				PublicIPv6: ip.IP,
				Public:     true,
				Service:    d.name(),
				Metadata:   metadata,
			})
		}
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}
		opt.Page = page + 1
	}
	return list, nil
}

func getReservedIPMetadata(region *godo.Region, droplet *godo.Droplet) map[string]string {
	metadata := make(map[string]string)

	if region != nil {
		schema.AddMetadata(metadata, "region_slug", &region.Slug)
	}
	if droplet != nil {
		dropletID := fmt.Sprintf("%d", droplet.ID)
		schema.AddMetadata(metadata, "droplet_id", &dropletID)
		schema.AddMetadata(metadata, "droplet_name", &droplet.Name)
	}
	return metadata
}
