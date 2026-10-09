package linode

import (
	"context"
	"net/url"

	"github.com/linode/linodego"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// lkeProvider is a Kubernetes (LKE) cluster provider for linode API
type lkeProvider struct {
	id     string
	client *linodego.Client
}

func (d *lkeProvider) name() string {
	return "lke"
}

// GetResource returns the control plane API endpoints of all LKE clusters.
func (d *lkeProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	clusters, err := d.client.ListLKEClusters(ctx, nil)
	if err != nil {
		return nil, err
	}

	list := schema.NewResources()
	for _, cluster := range clusters {
		endpoints, err := d.client.ListLKEClusterAPIEndpoints(ctx, cluster.ID, nil)
		if err != nil {
			// The API returns an error until a new cluster's control plane is
			// ready; skip it rather than failing the other clusters.
			continue
		}
		for _, endpoint := range endpoints {
			u, err := url.Parse(endpoint.Endpoint)
			if err != nil || u.Hostname() == "" {
				continue
			}
			list.Append(&schema.Resource{
				Provider: providerName,
				DNSName:  u.Hostname(),
				ID:       d.id,
				Public:   true,
				Service:  d.name(),
			})
		}
	}
	return list, nil
}
