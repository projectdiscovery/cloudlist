package linode

import (
	"context"
	"net/http"

	"github.com/linode/linodego"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"golang.org/x/oauth2"
)

const (
	apiKey       = "linode_personal_access_token"
	providerName = "linode"
)

// Provider is a data provider for linode API
type Provider struct {
	id     string
	client *linodego.Client
}

// New creates a new provider client for linode API
func New(options schema.OptionBlock) (*Provider, error) {
	apiKey, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiKey}
	}
	id, _ := options.GetMetadata("id")

	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: apiKey})
	oc := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
			Base:   nil,
		},
	}

	client := linodego.NewClient(oc)

	return &Provider{id: id, client: &client}, nil
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
