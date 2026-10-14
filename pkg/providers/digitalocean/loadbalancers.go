package digitalocean

import (
	"context"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// loadBalancerProvider is a load balancer provider for digitalocean API
type loadBalancerProvider struct {
	id               string
	client           *godo.Client
	extendedMetadata bool
}

func (d *loadBalancerProvider) name() string {
	return "loadbalancer"
}

// GetResource returns all the load balancer IPs and domains for a provider.
func (d *loadBalancerProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	opt := &godo.ListOptions{PerPage: 200}
	list := schema.NewResources()

	for {
		lbs, resp, err := d.client.LoadBalancers.List(ctx, opt)
		if err != nil {
			return list, err
		}

		for _, lb := range lbs {
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = d.getLoadBalancerMetadata(&lb)
			}

			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         d.id,
				PublicIPv4: lb.IP,
				PublicIPv6: lb.IPv6,
				Public:     true,
				Service:    d.name(),
				Metadata:   metadata,
			})
			// Global load balancers are reached through their domains, not an IP.
			for _, domain := range lb.Domains {
				if domain == nil {
					continue
				}
				list.Append(&schema.Resource{
					Provider: providerName,
					ID:       d.id,
					DNSName:  domain.Name,
					Public:   true,
					Service:  d.name(),
					Metadata: metadata,
				})
			}
		}
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return list, err
		}
		opt.Page = page + 1
	}
	return list, nil
}

func (d *loadBalancerProvider) getLoadBalancerMetadata(lb *godo.LoadBalancer) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "load_balancer_id", &lb.ID)
	schema.AddMetadata(metadata, "name", &lb.Name)
	schema.AddMetadata(metadata, "type", &lb.Type)
	schema.AddMetadata(metadata, "network", &lb.Network)
	schema.AddMetadata(metadata, "status", &lb.Status)
	schema.AddMetadata(metadata, "created_at", &lb.Created)
	schema.AddMetadata(metadata, "vpc_uuid", &lb.VPCUUID)
	schema.AddMetadata(metadata, "project_id", &lb.ProjectID)

	if lb.Region != nil {
		schema.AddMetadata(metadata, "region_slug", &lb.Region.Slug)
	}
	if len(lb.Tags) > 0 {
		tags := strings.Join(lb.Tags, ",")
		schema.AddMetadata(metadata, "tags", &tags)
	}
	schema.AddMetadataInt(metadata, "droplet_count", len(lb.DropletIDs))

	return metadata
}
