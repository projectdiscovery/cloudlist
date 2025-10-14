package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
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

var Services = []string{"vm", "publicip", "trafficmanager"}

// Provider is a data provider for Azure API using Track 2 SDK
type Provider struct {
	id               string
	SubscriptionIDs  []string
	Credential       azcore.TokenCredential // Track 2: replaced autorest.Authorizer
	services         schema.ServiceMap
	extendedMetadata bool
}

// New creates a new provider client for Azure API using Track 2 SDK
func New(options schema.OptionBlock) (*Provider, error) {
	ID, _ := options.GetMetadata(id)

	// Track 2: Create credential using new authentication layer
	credential, err := createCredential(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	gologger.Info().Msgf("Azure authentication method: %s", getAuthenticationSummary(options))

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
		Credential: credential, // Track 2: use credential instead of authorizer
		id:         ID,
		services:   services,
	}
	if extendedMetadata, ok := options.GetMetadata("extended_metadata"); ok {
		provider.extendedMetadata = extendedMetadata == "true"
	}

	// Check if a specific subscription ID was provided
	specifiedSubID, hasSpecificSub := options.GetMetadata(subscriptionID)

	// If a specific subscription was provided, use only that one
	if hasSpecificSub && specifiedSubID != "" {
		provider.SubscriptionIDs = []string{specifiedSubID}
		return provider, nil
	}

	// Otherwise, discover all available subscriptions using Track 2 SDK
	gologger.Info().Msgf("Listing subscriptions from provider: azure")

	ctx := context.Background()
	subsClient, err := armsubscriptions.NewClient(credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	var subIDs []string
	pager := subsClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list subscriptions: %w", err)
		}

		for _, sub := range page.Value {
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
			vmp := &vmProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			vmIPs, err := vmp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing VM public IPs for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(vmIPs)
		}

		if p.services.Has("publicip") {
			publicIPp := &publicIPProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			publicIPs, err := publicIPp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing public IPs for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(publicIPs)
		}

		if p.services.Has("trafficmanager") {
			trafficManagerp := &trafficManagerProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			trafficManager, err := trafficManagerp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing traffic manager for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(trafficManager)
		}
	}
	return resources, nil
}

// Verify checks if the provider is valid using simple API call with Track 2 SDK
func (p *Provider) Verify(ctx context.Context) error {
	// Simple verification: try to create a subscriptions client and list one subscription
	subsClient, err := armsubscriptions.NewClient(p.Credential, nil)
	if err != nil {
		return fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	// Try to list at least one subscription
	pager := subsClient.NewListPager(nil)
	if !pager.More() {
		return fmt.Errorf("no subscriptions found with provided credentials")
	}

	_, err = pager.NextPage(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify Azure credentials: %w", err)
	}

	gologger.Info().Msg("Azure credentials verified successfully")
	return nil
}
