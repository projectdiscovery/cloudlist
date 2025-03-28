package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/network/mgmt/network"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
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
	subscriptionID = `subscription_id` // optional
	useCliAuth     = `use_cli_auth`

	providerName = "azure"
)

var Services = []string{"vm", "publicip"}

// Provider is a data provider for Azure API
type Provider struct {
	id              string
	SubscriptionIDs []string
	Authorizer      autorest.Authorizer
	services        schema.ServiceMap
}

// New creates a new provider client for Azure API
func New(options schema.OptionBlock) (*Provider, error) {
	ID, _ := options.GetMetadata(id)
	UseCliAuth, _ := options.GetMetadata(useCliAuth)

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

	// Parse services
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

	provider := &Provider{
		Authorizer: authorizer,
		id:         ID,
		services:   services,
	}

	// Check if a specific subscription ID was provided
	specifiedSubID, hasSpecificSub := options.GetMetadata(subscriptionID)

	// If a specific subscription was provided, use only that one
	if hasSpecificSub && specifiedSubID != "" {
		provider.SubscriptionIDs = []string{specifiedSubID}
		return provider, nil
	}

	// Otherwise, discover all available subscriptions
	gologger.Info().Msgf("Listing subscriptions from provider: azure")

	ctx := context.Background()
	subsClient := subscriptions.NewClient()
	subsClient.Authorizer = authorizer

	var subIDs []string
	for subsList, err := subsClient.List(ctx); subsList.NotDone(); err = subsList.NextWithContext(ctx) {
		if err != nil {
			return nil, fmt.Errorf("failed to list subscriptions: %v", err)
		}

		for _, sub := range subsList.Values() {
			if sub.SubscriptionID != nil {
				subIDs = append(subIDs, *sub.SubscriptionID)
				gologger.Info().Msgf("Discovered subscription: %s", *sub.SubscriptionID)
			}
		}
	}

	if len(subIDs) == 0 {
		return nil, fmt.Errorf("no subscriptions found for the provided credentials")
	}

	provider.SubscriptionIDs = subIDs
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
	resources := schema.NewResources()

	// Process each subscription
	for _, subscriptionID := range p.SubscriptionIDs {
		gologger.Info().Msgf("Processing subscription: %s", subscriptionID)

		if p.services.Has("vm") {
			vmp := &vmProvider{Authorizer: p.Authorizer, SubscriptionID: subscriptionID, id: p.id}
			vmIPs, err := vmp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing VM public IPs for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(vmIPs)
		}

		if p.services.Has("publicip") {
			publicIPp := &publicIPProvider{Authorizer: p.Authorizer, SubscriptionID: subscriptionID, id: p.id}
			publicIPs, err := publicIPp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing public IPs for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(publicIPs)
		}
	}

	return resources, nil
}

// Verify checks if the provider is valid using simple API call
func (p *Provider) Verify(ctx context.Context) error {
	for _, subscriptionID := range p.SubscriptionIDs {
		groupsClient := resources.NewGroupsClient(subscriptionID)
		groupsClient.Authorizer = p.Authorizer

		pClient := network.NewPublicIPAddressesClient(subscriptionID)
		pClient.Authorizer = p.Authorizer

		// Try a lightweight operation - just list the first group
		var success bool
		if p.services.Has("vm") {
			_, err := groupsClient.List(ctx, "", nil)
			if err != nil {
				return fmt.Errorf("failed to verify Azure credentials: %v", err)
			}
			success = true
		} else if p.services.Has("publicip") && !success {
			_, err := pClient.ListAllComplete(ctx)
			if err != nil {
				return fmt.Errorf("failed to verify Azure credentials: %v", err)
			}
			success = true
		}
		if success {
			return nil
		}
		return fmt.Errorf("no accessible Azure services found with provided credentials")
	}
	return fmt.Errorf("no accessible Azure services found with provided credentials")
}
