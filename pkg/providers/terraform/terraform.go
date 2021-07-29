package terraform

import (
	"context"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

const (
	statePathFile = "tf_state_file"
	providerName  = "terraform"
)

// Provider is a data provider for Terraform
type Provider struct {
	profile string
	path    string
}

// New creates a new provider client for Terraform
func New(options schema.OptionBlock) (*Provider, error) {
	StatePathFile, ok := options.GetMetadata(statePathFile)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: statePathFile}
	}
	profile, _ := options.GetMetadata("profile")
	return &Provider{path: StatePathFile, profile: profile}, nil
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
	provider := &instanceProvider{path: p.path, profile: p.profile}
	return provider.GetResource(ctx)
}
