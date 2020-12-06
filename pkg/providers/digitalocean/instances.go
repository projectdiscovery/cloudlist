package digitalocean

import (
	"context"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for digitalocean API
type instanceProvider struct {
	profile string
	client  *godo.Client
}

// GetInstances returns all the instances in the store for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	opt := &godo.ListOptions{PerPage: 200}
	list := &schema.Resources{}

	for {
		droplets, resp, err := d.client.Droplets.List(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, droplet := range droplets {
			ip4, _ := droplet.PublicIPv4()
			privateIP4, _ := droplet.PrivateIPv4()

			list.Append(&schema.Resource{
				Provider:    providerName,
				PublicIPv4:  ip4,
				Profile:     d.profile,
				PrivateIpv4: privateIP4,
				Public:      ip4 != "",
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
