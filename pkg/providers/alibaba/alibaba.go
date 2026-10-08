package alibaba

import (
	"context"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"instance", "slb", "alb", "eip", "ack"}

const (
	regionID        = "alibaba_region_id"
	accessKeyID     = "alibaba_access_key"
	accessKeySecret = "alibaba_access_key_secret"
	providerName    = "alibaba"
	pageSize        = 100
)

// Provider is a data provider for alibaba API
type Provider struct {
	id        string
	ecsClient *ecs.Client
	slbClient *slb.Client
	albClient *alb.Client
	vpcClient *vpc.Client
	csClient  *cs.Client
	services  schema.ServiceMap
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
	provider := &Provider{id: id}

	services := options.ResolveServices(Services)
	provider.services = services

	// The SDK defaults to plain HTTP.
	config := sdk.NewConfig().WithScheme("HTTPS")
	credential := credentials.NewAccessKeyCredential(accessKeyID, accessKeySecret)
	var err error
	if services.Has("instance") {
		if provider.ecsClient, err = ecs.NewClientWithOptions(regionID, config, credential); err != nil {
			return nil, err
		}
	}
	if services.Has("slb") {
		if provider.slbClient, err = slb.NewClientWithOptions(regionID, config, credential); err != nil {
			return nil, err
		}
	}
	if services.Has("alb") {
		if provider.albClient, err = alb.NewClientWithOptions(regionID, config, credential); err != nil {
			return nil, err
		}
	}
	if services.Has("eip") {
		if provider.vpcClient, err = vpc.NewClientWithOptions(regionID, config, credential); err != nil {
			return nil, err
		}
	}
	if services.Has("ack") {
		if provider.csClient, err = cs.NewClientWithOptions(regionID, config, credential); err != nil {
			return nil, err
		}
	}

	return provider, nil
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

	var providers []interface {
		GetResource(ctx context.Context) (*schema.Resources, error)
	}
	if p.ecsClient != nil {
		providers = append(providers, &instanceProvider{client: p.ecsClient, id: p.id})
	}
	if p.slbClient != nil {
		providers = append(providers, &slbProvider{client: p.slbClient, id: p.id})
	}
	if p.albClient != nil {
		providers = append(providers, &albProvider{client: p.albClient, id: p.id})
	}
	if p.vpcClient != nil {
		providers = append(providers, &eipProvider{client: p.vpcClient, id: p.id})
	}
	if p.csClient != nil {
		providers = append(providers, &ackProvider{client: p.csClient, id: p.id})
	}

	for _, provider := range providers {
		if resources, err := provider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
