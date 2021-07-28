package terraform

import (
	"context"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// Provider is a data provider for Terraform API
type Provider struct {
	profile string
	path    string
}

// New creates a new provider client for Terraform API
func New(options schema.OptionBlock) (*Provider, error) {
	statePathFile, ok := options.GetMetadata(statePath)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: statePath}
	}
	profile, _ := options.GetMetadata("profile")
	return &Provider{path: statePathFile, profile: profile}, nil
}

const providerName = "terraform"

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ProfileName returns the name of the provider profile
func (p *Provider) ProfileName() string {
	return p.profile
}

const statePath = "path"

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	provider := &instanceProvider{path: p.path, profile: p.profile}
	return provider.GetResource(ctx)
}
