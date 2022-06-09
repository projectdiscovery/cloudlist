package hetzner

import (
	"context"
	hetzner "github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

const (
	authToken    = "auth_token"
	providerName = "hetzner"
)

// Provider is a data provider for Hetzner Cloud API
type Provider struct {
	id     string
	client *hetzner.Client
}

// New creates a new provider client for Hetzner Cloud API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(authToken)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: authToken}
	}

	id, _ := options.GetMetadata("id")
	opts := hetzner.WithToken(token)
	return &Provider{id: id, client: hetzner.NewClient(opts)}, nil
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

// Resources returns the provider for an resource
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	provider := &instanceProvider{client: p.client, id: p.id}
	return provider.GetResource(ctx)
}
