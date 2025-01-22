package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/network/mgmt/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// publicIPProvider is a public ip provider for Azure API
type publicIPProvider struct {
	id             string
	SubscriptionID string
	Authorizer     autorest.Authorizer
}

func (pip *publicIPProvider) name() string {
	return "publicip"
}

// GetResource returns all the resources in the store for a provider.
func (pip *publicIPProvider) GetResource(ctx context.Context) (*schema.Resources, error) {

	list := schema.NewResources()

	ips, err := pip.fetchPublicIPs(ctx)
	if err != nil {
		return nil, err
	}

	for _, ip := range ips {
		// The IPAddress field can be nil and so we want to prevent from dereferencing
		// a nil field in the struct
		if ip.IPAddress == nil {
			continue
		}

		resource := &schema.Resource{
			Provider: providerName,
			ID:       pip.id,
			Public:   true,
			Service:  pip.name(),
		}

		if ip.PublicIPAddressVersion == network.IPv4 {
			resource.PublicIPv4 = *ip.IPAddress
		} else {
			resource.PublicIPv6 = *ip.IPAddress
		}

		list.Append(resource)
	}
	return list, nil
}

func (pip *publicIPProvider) fetchPublicIPs(ctx context.Context) ([]network.PublicIPAddress, error) {
	var ips []network.PublicIPAddress

	ipClient := network.NewPublicIPAddressesClient(pip.SubscriptionID)
	ipClient.Authorizer = pip.Authorizer

	ipsIt, err := ipClient.ListAllComplete(ctx)
	if err != nil {
		return nil, err
	}

	for ipsIt.NotDone() {
		ip := ipsIt.Value()
		ips = append(ips, ip)

		if err = ipsIt.NextWithContext(ctx); err != nil {
			return ips, err
		}
	}

	return ips, err
}
