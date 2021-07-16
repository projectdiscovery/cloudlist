package cloudflare

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// Provider is a data provider for cloudflare API
type Provider struct {
	profile string
	client  *cloudflare.API
}

// New creates a new provider client for cloudflare API
func New(options schema.OptionBlock) (*Provider, error) {
	accessKey, ok := options.GetMetadata(apiAccessKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	apiEmail, ok := options.GetMetadata(apiEmail)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiEmail}
	}
	profile, _ := options.GetMetadata("profile")

	// Construct a new API object
	api, err := cloudflare.New(accessKey, apiEmail)
	if err != nil {
		return nil, err
	}
	return &Provider{profile: profile, client: api}, nil
}

const apiAccessKey = "api_key"
const apiEmail = "email"
const providerName = "cloudflare"

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
	dnsProvider := &dnsProvider{profile: p.profile, client: p.client}
	list, err := dnsProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	return list, nil
}
