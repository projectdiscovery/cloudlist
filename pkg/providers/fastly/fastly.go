package fastly

import (
	"context"
	"log"

	"github.com/fastly/go-fastly/v3/fastly"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

const (
	apiKey       = "fastly_api_key"
	providerName = "fastly"
)

// Provider is a data provider for fastly API
type Provider struct {
	client  *fastly.Client
	profile string
}

// New creates a new provider client for fastly API
func New(options schema.OptionBlock) (*Provider, error) {
	apiKey, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, errors.New("could not get API Key")
	}
	profile, _ := options.GetMetadata("profile")

	client, err := fastly.NewClient(apiKey)
	if err != nil {
		log.Fatal(err)
	}
	return &Provider{client: client, profile: profile}, err
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
	serviceProvider := &serviceProvider{client: p.client, profile: p.profile}
	services, err := serviceProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	return services, nil
}
