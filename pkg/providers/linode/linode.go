package linode

import (
	"context"
	"net/http"

	"github.com/linode/linodego"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"golang.org/x/oauth2"
)

var Services = []string{"instance"}

const (
	apiKey       = "linode_personal_access_token"
	providerName = "linode"
)

// Provider is a data provider for linode API
type Provider struct {
	id       string
	client   *linodego.Client
	services schema.ServiceMap
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

	services := options.ResolveServices(Services)
	return &Provider{id: id, client: &client, services: services}, nil
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

// Services returns the provider services
func (p *Provider) Services() []string {
	return p.services.Keys()
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()
	if p.services.Has("instance") {
		provider := &instanceProvider{client: p.client, id: p.id}
		if resources, err := provider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
