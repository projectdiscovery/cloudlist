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
	profile string
	client  *heroku.Service
}

// New creates a new provider client for Heroku API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiKey}
	}
	profile, _ := options.GetMetadata("profile")

	heroku.DefaultTransport.BearerToken = token

	return &Provider{profile: profile, client: heroku.NewService(heroku.DefaultClient)}, nil
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ProfileName returns the name of the provider profile
func (p *Provider) ProfileName() string {
	return p.profile
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	provider := &instanceProvider{client: p.client, profile: p.profile}
	return provider.GetResource(ctx)
}
