package alibaba

import (
	"context"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"instance"}

const (
	regionID        = "alibaba_region_id"
	accessKeyID     = "alibaba_access_key"
	accessKeySecret = "alibaba_access_key_secret"
	providerName    = "alibaba"
)

// Provider is a data provider for alibaba API
type Provider struct {
	id        string
	ecsClient *ecs.Client
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

	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}
	services := make(schema.ServiceMap)
	if ss, ok := options.GetMetadata("services"); ok {
		for _, s := range strings.Split(ss, ",") {
			if _, ok := supportedServicesMap[s]; ok {
				services[s] = struct{}{}
			}
		}
	}
	if len(services) == 0 {
		for _, s := range Services {
			services[s] = struct{}{}
		}
	}
	provider.services = services

	if services.Has("instance") {
		client, err := ecs.NewClientWithAccessKey(
			regionID,        // region ID
			accessKeyID,     // AccessKey ID
			accessKeySecret, // AccessKey secret
		)
		if err != nil {
			return nil, err
		}
		provider.ecsClient = client
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
	if p.ecsClient != nil {
		ecsprovider := &instanceProvider{client: p.ecsClient, id: p.id}
		if resources, err := ecsprovider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
