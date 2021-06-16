package heroku

import (
	"context"

	heroku "github.com/heroku/heroku-go/v5"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for Heroku API
type instanceProvider struct {
	profile string
	client  *heroku.Service
}

// GetResource returns all the applications for the Heroku provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := &schema.Resources{}

	apps, err := d.client.AppList(context.TODO(), &heroku.ListRange{Field: "name"})
	if err != nil {
		return nil, err
	}

	var isPlublic bool

	for _, app := range apps {
		isPlublic = true
		if app.InternalRouting != nil {
			isPlublic = !(*app.InternalRouting)
		}

		list.Append(&schema.Resource{
			DNSName:  app.WebURL,
			Public:   isPlublic,
			Profile:  d.profile,
			Provider: providerName,
		})
	}

	return list, nil
}
