package digitalocean

import (
	"context"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// Provider is a data provider for digitalocean API
type Provider struct {
	id     string
	client *godo.Client
}

// New creates a new provider client for digitalocean API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiKey}
	}
	id, _ := options.GetMetadata("id")
	return &Provider{id: id, client: godo.NewFromToken(token)}, nil
}

const providerName = "digitalocean"

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

const apiKey = "digitalocean_token"

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	instanceprovider := &instanceProvider{client: p.client, id: p.id}
	instances, err := instanceprovider.GetResource(ctx)
	if err != nil {
		return nil, err
	}

	appprovider := &appsProvider{client: p.client, id: p.id}
	apps, err := appprovider.GetResource(ctx)
	if err != nil {
		return nil, err
	}

	finalList := schema.NewResources()
	finalList.Merge(instances)
	finalList.Merge(apps)
	return finalList, nil
}
