package k8s

import (
	"context"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	v1 "k8s.io/api/core/v1"
)

// K8sServiceProvider is a provider for aws Route53 API
type K8sServiceProvider struct {
	id            string
	serviceClient *v1.ServiceList
}

func (k *K8sServiceProvider) name() string {
	return "service"
}

// GetResource returns all the resources in the store for a provider.
func (k *K8sServiceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	for _, service := range k.serviceClient.Items {
		if service.Spec.LoadBalancerIP != "" {
			list.Append(&schema.Resource{
				Public:     true,
				Provider:   providerName,
				ID:         k.id,
				PublicIPv4: service.Spec.LoadBalancerIP,
				Service:    k.name(),
			})
		}
		if service.Spec.Type == "LoadBalancer" {
			for _, ip := range service.Status.LoadBalancer.Ingress {
				list.Append(&schema.Resource{
					Public:     true,
					Provider:   providerName,
					ID:         k.id,
					PublicIPv4: ip.IP,
					DNSName:    ip.Hostname,
					Service:    "load_balancer",
				})
			}
		}
		for _, ip := range service.Spec.ExternalIPs {
			list.Append(&schema.Resource{
				Public:      true,
				Provider:    providerName,
				ID:          k.id,
				PublicIPv4:  ip,
				PrivateIpv4: "",
				DNSName:     "",
				Service:     "external_ip",
			})
		}
		for _, ip := range service.Spec.ClusterIPs {
			if ip == "None" {
				continue
			}
			list.Append(&schema.Resource{
				Public:      false,
				Provider:    providerName,
				ID:          k.id,
				PrivateIpv4: ip,
				Service:     "cluster_ip",
			})
		}
	}
	return list, nil
}
