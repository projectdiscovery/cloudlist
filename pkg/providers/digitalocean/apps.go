package digitalocean

import (
	"context"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// appsProvider is an instance provider for digitalocean API
type appsProvider struct {
	id     string
	client *godo.Client
}

func (d *appsProvider) name() string {
	return "app"
}

// GetInstances returns all the instances in the store for a provider.
func (d *appsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	opt := &godo.ListOptions{PerPage: 200}
	list := schema.NewResources()

	for {
		apps, resp, err := d.client.Apps.List(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, app := range apps {
			dnsname := app.LiveDomain

			list.Append(&schema.Resource{
				Provider: providerName,
				ID:       d.id,
				DNSName:  dnsname,
				Public:   true,
				Service:  d.name(),
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
