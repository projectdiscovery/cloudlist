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

var Services = []string{"vm", "publicip", "trafficmanager", "cdn", "dns", "loadbalancer", "applicationgateway", "aks", "storage", "containerinstances", "appservice", "functions", "apimanagement", "frontdoor", "containerapps", "staticwebapps"}

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

	// Parse services
	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}
	services := make(schema.ServiceMap)
	if ss, ok := options.GetMetadata("services"); ok {
		for _, s := range strings.Split(ss, ",") {
			s = strings.TrimSpace(s)
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

		if p.services.Has("cdn") {
			cdnp := &cdnProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			cdn, err := cdnp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing CDN for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(cdn)
		}

		if p.services.Has("dns") {
			dnsp := &dnsProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			dnsRecords, err := dnsp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing DNS records for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(dnsRecords)
		}

		if p.services.Has("loadbalancer") {
			lbp := &loadBalancerProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			loadBalancers, err := lbp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing load balancers for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(loadBalancers)
		}

		if p.services.Has("applicationgateway") {
			agp := &applicationGatewayProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			appGateways, err := agp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing application gateways for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(appGateways)
		}

		if p.services.Has("aks") {
			aksp := &aksProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			aksClusters, err := aksp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing AKS clusters for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(aksClusters)
		}

		if p.services.Has("storage") {
			storagep := &storageProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			storageAccounts, err := storagep.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing storage accounts for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(storageAccounts)
		}

		if p.services.Has("containerinstances") {
			containerInstancesp := &containerInstancesProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			containerInstances, err := containerInstancesp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing container instances for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(containerInstances)
		}

		if p.services.Has("appservice") {
			appServicep := &appServiceProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			appService, err := appServicep.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing App Service for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(appService)
		}

		if p.services.Has("functions") {
			functionsp := &functionsProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			functions, err := functionsp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing Functions for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(functions)
		}

		if p.services.Has("apimanagement") {
			apimgmtp := &apiManagementProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			apiManagement, err := apimgmtp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing API Management for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(apiManagement)
		}

		if p.services.Has("containerapps") {
			containerAppsp := &containerAppsProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			containerApps, err := containerAppsp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing container apps for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(containerApps)
		}

		if p.services.Has("staticwebapps") {
			staticWebAppsp := &staticWebAppsProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			staticWebApps, err := staticWebAppsp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing static web apps for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(staticWebApps)
		}

		if p.services.Has("frontdoor") {
			fdp := &frontDoorProvider{Credential: p.Credential, SubscriptionID: subscriptionID, id: p.id, extendedMetadata: p.extendedMetadata}
			frontDoors, err := fdp.GetResource(ctx)
			if err != nil {
				gologger.Warning().Msgf("Error listing Front Door for subscription %s: %s", subscriptionID, err)
				continue
			}
			resources.Merge(frontDoors)
		}
	}
	return resources, nil
}

// Verify checks if the provider is valid using simple API call with Track 2 SDK
func (p *Provider) Verify(ctx context.Context) error {
	// Simple verification: try to create a subscriptions client and ensure at least one subscription exists
	subsClient, err := armsubscriptions.NewClient(p.Credential, nil)
	if err != nil {
		return fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	pager := subsClient.NewListPager(nil)
	found := false
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to verify Azure credentials: %w", err)
		}
		if len(page.Value) > 0 {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no subscriptions found with provided credentials")
	}

	return nil
}
