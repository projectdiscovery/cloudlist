package digitalocean

import (
	"context"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for digitalocean API
type instanceProvider struct {
	id     string
	client *godo.Client
}

// GetInstances returns all the instances in the store for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	opt := &godo.ListOptions{PerPage: 200}
	list := schema.NewResources()

	for {
		droplets, resp, err := d.client.Droplets.List(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, droplet := range droplets {
			ip4, _ := droplet.PublicIPv4()
			privateIP4, _ := droplet.PrivateIPv4()

			if privateIP4 != "" {
				list.Append(&schema.Resource{
					Provider:    providerName,
					ID:          d.id,
					PrivateIpv4: privateIP4,
				})
			}
			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         d.id,
				PublicIPv4: ip4,
				Public:     true,
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
