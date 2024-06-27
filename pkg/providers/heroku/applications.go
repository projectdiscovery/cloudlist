package heroku

import (
	"context"

	heroku "github.com/heroku/heroku-go/v5"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for Heroku API
type instanceProvider struct {
	id     string
	client *heroku.Service
}

func (d *instanceProvider) name() string {
	return "application"
}

// GetResource returns all the applications for the Heroku provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	apps, err := d.client.AppList(ctx, &heroku.ListRange{Field: "id", Max: 1000})
	if err != nil {
		return nil, err
	}

	var isPublic bool

	for _, app := range apps {
		isPublic = true
		if app.InternalRouting != nil {
			isPublic = !(*app.InternalRouting)
		}

		list.Append(&schema.Resource{
			DNSName:  app.WebURL,
			Public:   isPublic,
			ID:       d.id,
			Provider: providerName,
			Service:  d.name(),
		})
	}

	return list, nil
}
