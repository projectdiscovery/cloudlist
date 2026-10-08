package linode

import (
	"context"
	"strings"

	"github.com/linode/linodego"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for linode API
type instanceProvider struct {
	id     string
	client *linodego.Client
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetResource returns all the instance resources for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {

	// Using autop-agination as mentioned by https://github.com/linode/linodego#auto-pagination-requests
	// We can also use handle pagination manually if needed
	instances, err := d.client.ListInstances(ctx, nil)
	if err != nil {
		return nil, err
	}

	list := schema.NewResources()

	for _, inst := range instances {
		// Assuming (and obseved the same) first IP in the list is the public IP
		ip4 := inst.IPv4[0].String()
		// The API returns the IPv6 address with its prefix length (e.g. "/128").
		ip6, _, _ := strings.Cut(inst.IPv6, "/")

		list.Append(&schema.Resource{
			Provider:   providerName,
			PublicIPv4: ip4,
			PublicIPv6: ip6,
			ID:         d.id,
			Public:     ip4 != "",
			Service:    d.name(),
		})
	}

	return list, nil
}
