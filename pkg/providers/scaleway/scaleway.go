package scaleway

import (
	"context"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// Provider is a data provider for scaleway API
type Provider struct {
	id     string
	client *scw.Client
}

// New creates a new provider client for scaleway API
func New(options schema.OptionBlock) (*Provider, error) {
	accessKey, ok := options.GetMetadata(apiAccessKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	accessToken, ok := options.GetMetadata(apiAccessToken)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessToken}
	}
	id, _ := options.GetMetadata("id")

	client, err := scw.NewClient(scw.WithAuth(accessKey, accessToken))
	if err != nil {
		return nil, err
	}
	return &Provider{client: client, id: id}, nil
}

const providerName = "scw"

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

const apiAccessKey = "scaleway_access_key"
const apiAccessToken = "scaleway_access_token"

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	provider := &instanceProvider{instanceAPI: instance.NewAPI(p.client), id: p.id}
	return provider.GetResource(ctx)
}
