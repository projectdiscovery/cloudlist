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
	id   string
	path string
}

// New creates a new provider client for Terraform
func New(options schema.OptionBlock) (*Provider, error) {
	StatePathFile, ok := options.GetMetadata(statePathFile)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: statePathFile}
	}
	id, _ := options.GetMetadata("id")
	return &Provider{path: StatePathFile, id: id}, nil
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
	provider := &instanceProvider{path: p.path, id: p.id}
	return provider.GetResource(ctx)
}
