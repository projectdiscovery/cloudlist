package alibaba

import (
	"context"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

const (
	regionID        = "alibaba_region_id"
	accessKeyID     = "alibaba_access_key"
	accessKeySecret = "alibaba_access_key_secret"
	providerName    = "alibaba"
)

// Provider is a data provider for alibaba API
type Provider struct {
	id     string
	client *ecs.Client
}

// New creates a new provider client for alibaba API
func New(options schema.OptionBlock) (*Provider, error) {
	regionID, ok := options.GetMetadata(regionID)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: regionID}
	}
	accessKeyID, ok := options.GetMetadata(accessKeyID)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: accessKeyID}
	}
	accessKeySecret, ok := options.GetMetadata(accessKeySecret)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: accessKeySecret}
	}

	id, _ := options.GetMetadata("id")

	client, err := ecs.NewClientWithAccessKey(
		regionID,        // region ID
		accessKeyID,     // AccessKey ID
		accessKeySecret, // AccessKey secret
	)
	if err != nil {
		return nil, err
	}

	return &Provider{client: client, id: id}, nil
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
	ecsprovider := &instanceProvider{client: p.client, id: p.id}
	list, err := ecsprovider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	return list, nil
}
