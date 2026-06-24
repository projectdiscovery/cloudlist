package cloudflare

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"dns"}

// Provider is a data provider for cloudflare API
type Provider struct {
	id               string
	client           apiClient
	services         schema.ServiceMap
	extendedMetadata bool
}

// New creates a new provider client for cloudflare API
// Here api_token overrides api_key
func New(options schema.OptionBlock) (*Provider, error) {
	id, _ := options.GetMetadata("id")

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
	if es, ok := options.GetMetadata("exclude_services"); ok {
		for _, s := range strings.Split(es, ",") {
			delete(services, strings.TrimSpace(s))
		}
	}

	// Parse extended metadata option
	extendedMetadata := false
	if extMetadata, ok := options.GetMetadata("extended_metadata"); ok {
		extendedMetadata = extMetadata == "true"
	}

	apiToken, ok := options.GetMetadata(apiToken)
	if ok {
		// Construct a new API object with scoped api token
		api, err := cloudflare.NewWithAPIToken(apiToken)
		if err != nil {
			return nil, err
		}
		return &Provider{id: id, client: api, services: services, extendedMetadata: extendedMetadata}, nil
	}

	accessKey, ok := options.GetMetadata(apiAccessKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	apiEmail, ok := options.GetMetadata(apiEmail)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiEmail}
	}

	// Construct a new API object
	api, err := cloudflare.New(accessKey, apiEmail)
	if err != nil {
		return nil, err
	}

	return &Provider{id: id, client: api, services: services, extendedMetadata: extendedMetadata}, nil
}

// apiToken is a cloudflare scoped API token
const apiToken = "api_token"
const apiAccessKey = "api_key"
const apiEmail = "email"
const providerName = "cloudflare"

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

// Verify checks if the provider credentials are valid using minimal API calls.
func (p *Provider) Verify(ctx context.Context) error {
	if !p.services.Has("dns") {
		return nil
	}

	zones, err := p.client.ListZones(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify Cloudflare zone access: %w", err)
	}

	if len(zones) == 0 {
		return fmt.Errorf("no accessible Cloudflare zones found with provided credentials")
	}

	_, _, err = p.client.ListDNSRecords(ctx, cloudflare.ZoneIdentifier(zones[0].ID), cloudflare.ListDNSRecordsParams{
		ResultInfo: cloudflare.ResultInfo{
			Page:    1,
			PerPage: 1,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to verify Cloudflare DNS access for zone %s: %w", zones[0].Name, err)
	}

	return nil
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()

	if p.services.Has("dns") {
		dnsProvider := &dnsProvider{id: p.id, client: p.client, extendedMetadata: p.extendedMetadata}
		resources, err := dnsProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(resources)
	}
	return finalResources, nil
}
