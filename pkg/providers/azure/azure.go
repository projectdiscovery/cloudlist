package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

const (
	id             = `id`
	tenantID       = `tenant_id`
	clientID       = `client_id`
	clientSecret   = `client_secret`
	subscriptionID = `subscription_id`
	useCliAuth     = `use_cli_auth`

	providerName = "azure"
)

var Services = []string{"vm", "publicip"}

// Provider is a data provider for Azure API
type Provider struct {
	id             string
	SubscriptionID string
	Authorizer     autorest.Authorizer
	services       schema.ServiceMap
}

// New creates a new provider client for Azure API
func New(options schema.OptionBlock) (*Provider, error) {
	SubscriptionID, ok := options.GetMetadata(subscriptionID)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: subscriptionID}
	}

	UseCliAuth, _ := options.GetMetadata(useCliAuth)

	ID, _ := options.GetMetadata(id)

	var authorizer autorest.Authorizer
	var err error

	if UseCliAuth == "true" {
		authorizer, err = auth.NewAuthorizerFromCLI()
		if err != nil {
			gologger.Error().Msgf("Couldn't authorize using cli: %s\n", err)
			return nil, err
		}
	} else {
		ClientID, ok := options.GetMetadata(clientID)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: clientID}
		}
		ClientSecret, ok := options.GetMetadata(clientSecret)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: clientSecret}
		}
		TenantID, ok := options.GetMetadata(tenantID)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: tenantID}
		}

		config := auth.NewClientCredentialsConfig(ClientID, ClientSecret, TenantID)
		authorizer, err = config.Authorizer()
		if err != nil {
			return nil, err
		}
	}

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
	return &Provider{Authorizer: authorizer, SubscriptionID: SubscriptionID, id: ID, services: services}, nil

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
	resources := schema.NewResources()

	if p.services.Has("vm") {
		vmp := &vmProvider{Authorizer: p.Authorizer, SubscriptionID: p.SubscriptionID, id: p.id}
		vmIPs, err := vmp.GetResource(ctx)
		if err != nil {
			return nil, fmt.Errorf("error listing VM public ips: %s", err)
		}
		resources.Merge(vmIPs)
	}

	if p.services.Has("publicip") {

		publicIPp := &publicIPProvider{Authorizer: p.Authorizer, SubscriptionID: p.SubscriptionID, id: p.id}
		publicIPs, err := publicIPp.GetResource(ctx)
		if err != nil {
			return nil, fmt.Errorf("error listing public ips: %s", err)
		}
		resources.Merge(publicIPs)
	}
	return resources, nil
}
