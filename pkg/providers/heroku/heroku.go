package heroku

import (
	"context"

	heroku "github.com/heroku/heroku-go/v5"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

const (
	apiKey       = "heroku_api_token"
	providerName = "heroku"
)

// Provider is a data provider for Heroku API
type Provider struct {
	id     string
	client *heroku.Service
}

// New creates a new provider client for Heroku API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiKey}
	}
	id, _ := options.GetMetadata("id")

	heroku.DefaultTransport.BearerToken = token

	return &Provider{id: id, client: heroku.NewService(heroku.DefaultClient)}, nil
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	provider := &instanceProvider{client: p.client, id: p.id}
	return provider.GetResource(ctx)
}
