package k8s

import (
	"context"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	v1 "k8s.io/api/networking/v1"
)

// k8sServiceProvider is a provider for aws Route53 API
type K8sIngressProvider struct {
	id      string
	ingress *v1.IngressList
}

func NewK8sIngressProvider(id string, ingress *v1.IngressList) *K8sIngressProvider {
	return &K8sIngressProvider{
		id:      id,
		ingress: ingress,
	}
}

// GetResource returns all the resources in the store for a provider.
func (k *K8sIngressProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	for _, ingress := range k.ingress.Items {
		for _, rule := range ingress.Spec.Rules {
			// for _, path := range rule.IngressRuleValue.HTTP.Paths {
			// 	list.Append(&schema.Resource{
			// 		Public:   true,
			// 		Provider: providerName,
			// 		ID:       k.id,
			// 		DNSName:  fmt.Sprintf("%s%s", rule.Host, path.Path),
			// 	})
			// }
			list.Append(&schema.Resource{
				Public:   true,
				Provider: providerName,
				ID:       k.id,
				DNSName:  rule.Host,
			})
		}
		for _, ip := range ingress.Status.LoadBalancer.Ingress {
			if ip.IP == "" {
				list.Append(&schema.Resource{
					Public:     true,
					Provider:   providerName,
					ID:         k.id,
					PublicIPv4: ip.IP,
				})
			}
			if ip.Hostname == "" {
				list.Append(&schema.Resource{
					Public:   true,
					Provider: providerName,
					ID:       k.id,
					DNSName:  ip.Hostname,
				})
			}
		}
	}
	return list, nil
}
