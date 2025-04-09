package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/trafficmanager/mgmt/trafficmanager"
	"github.com/Azure/go-autorest/autorest"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// trafficManagerProvider is a provider for Azure Traffic Manager Profiles
type trafficManagerProvider struct {
	id             string
	SubscriptionID string
	Authorizer     autorest.Authorizer
}

// name returns the name of the provider
func (tmp *trafficManagerProvider) name() string {
	return "trafficmanager"
}

// GetResource returns all the Traffic Manager hostnames for a provider.
func (tmp *trafficManagerProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	profiles, err := tmp.fetchTrafficManagerProfiles(ctx)
	if err != nil {
		return nil, err
	}

	for _, profile := range *profiles {
		if profile.ProfileProperties != nil && profile.ProfileProperties.DNSConfig != nil && profile.ProfileProperties.DNSConfig.Fqdn != nil {
			resource := &schema.Resource{
				Provider: providerName,
				ID:       tmp.id,
				DNSName:  *profile.ProfileProperties.DNSConfig.Fqdn,
				Service:  tmp.name(),
			}
			list.Append(resource)
		}
	}
	return list, nil
}

// fetchTrafficManagerProfiles retrieves all Traffic Manager profiles for the subscription.
func (tmp *trafficManagerProvider) fetchTrafficManagerProfiles(ctx context.Context) (*[]trafficmanager.Profile, error) {
	client := trafficmanager.NewProfilesClient(tmp.SubscriptionID)
	client.Authorizer = tmp.Authorizer

	profilesIt, err := client.ListBySubscription(ctx)
	if err != nil {
		return nil, err
	}

	return profilesIt.Value, nil
}
