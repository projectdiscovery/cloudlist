package k8s

import (
	"context"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	v1 "k8s.io/api/core/v1"
)

// k8sServiceProvider is a provider for aws Route53 API
type k8sServiceProvider struct {
	id            string
	serviceClient *v1.ServiceList
}

// GetResource returns all the resources in the store for a provider.
func (d *k8sServiceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	for _, service := range d.serviceClient.Items {
		if service.Spec.LoadBalancerIP != "" {
			list.Append(&schema.Resource{
				Public:     true,
				Provider:   providerName,
				ID:         d.id,
				PublicIPv4: service.Spec.LoadBalancerIP,
			})
		}
		if service.Spec.Type == "LoadBalancer" {
			for _, ip := range service.Status.LoadBalancer.Ingress {
				list.Append(&schema.Resource{
					Public:     true,
					Provider:   providerName,
					ID:         d.id,
					PublicIPv4: ip.IP,
					DNSName:    ip.Hostname,
				})
			}
		}
		for _, ip := range service.Spec.ExternalIPs {
			list.Append(&schema.Resource{
				Public:      true,
				Provider:    providerName,
				ID:          d.id,
				PublicIPv4:  ip,
				PrivateIpv4: "",
				DNSName:     "",
			})
		}
		for _, ip := range service.Spec.ClusterIPs {
			list.Append(&schema.Resource{
				Public:      false,
				Provider:    providerName,
				ID:          d.id,
				PrivateIpv4: ip,
			})
		}
	}
	return list, nil
}
