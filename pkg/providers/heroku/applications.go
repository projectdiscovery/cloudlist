package heroku

import (
	"context"
	"encoding/json"
	"os"

	heroku "github.com/heroku/heroku-go/v5"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for Heroku API
type instanceProvider struct {
	profile string
	client  *heroku.Service
}

// GetInstances returns all the instances in the store for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := &schema.Resources{}

	apps, err := d.client.AppList(context.TODO(), &heroku.ListRange{Field: "name"})
	if err != nil {
		return nil, err
	}
	en := json.NewEncoder(os.Stdout)
	en.SetIndent(" ", " ")
	en.Encode(apps)

	return list, nil
}
