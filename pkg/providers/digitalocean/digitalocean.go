package digitalocean

import (
	"context"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// Provider is a data provider for digitalocean API
type Provider struct {
	profile string
	client  *godo.Client
}

// New creates a new provider client for digitalocean API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiKey}
	}
	profile, _ := options.GetMetadata("profile")
	return &Provider{profile: profile, client: godo.NewFromToken(token)}, nil
}

const providerName = "do"

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ProfileName returns the name of the provider profile
func (p *Provider) ProfileName() string {
	return p.profile
}

const apiKey = "digitalocean_token"

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	provider := &instanceProvider{client: p.client, profile: p.profile}
	return provider.GetResource(ctx)
}
