package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	v1 "k8s.io/api/networking/v1"
)

// k8sServiceProvider is a provider for aws Route53 API
type K8sIngressProvider struct {
	id               string
	ingress          *v1.IngressList
	extendedMetadata bool
}

func NewK8sIngressProvider(id string, ingress *v1.IngressList, extendedMetadata bool) *K8sIngressProvider {
	return &K8sIngressProvider{
		id:               id,
		ingress:          ingress,
		extendedMetadata: extendedMetadata,
	}
}

func (k *K8sIngressProvider) name() string {
	return "ingress"
}

// GetResource returns all the resources in the store for a provider.
func (k *K8sIngressProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	for _, ingress := range k.ingress.Items {
		var metadata map[string]string
		if k.extendedMetadata {
			metadata = k.getIngressMetadata(&ingress)
		}

		for _, rule := range ingress.Spec.Rules {
			list.Append(&schema.Resource{
				Public:   true,
				Provider: providerName,
				ID:       k.id,
				DNSName:  rule.Host,
				Service:  k.name(),
				Metadata: metadata,
			})
		}
		for _, ip := range ingress.Status.LoadBalancer.Ingress {
			if ip.IP != "" {
				list.Append(&schema.Resource{
					Public:     true,
					Provider:   providerName,
					ID:         k.id,
					PublicIPv4: ip.IP,
					Service:    k.name(),
					Metadata:   metadata,
				})
			}
			if ip.Hostname != "" {
				list.Append(&schema.Resource{
					Public:   true,
					Provider: providerName,
					ID:       k.id,
					DNSName:  ip.Hostname,
					Service:  k.name(),
					Metadata: metadata,
				})
			}
		}
	}
	return list, nil
}

func (k *K8sIngressProvider) getIngressMetadata(ingress *v1.Ingress) map[string]string {
	metadata := make(map[string]string)

	// Basic ingress information
	metadata["ingress_name"] = ingress.Name
	metadata["namespace"] = ingress.Namespace
	metadata["uid"] = string(ingress.UID)
	metadata["resource_version"] = ingress.ResourceVersion

	if ingress.CreationTimestamp.Time != (time.Time{}) {
		metadata["creation_timestamp"] = ingress.CreationTimestamp.Time.Format(time.RFC3339)
	}
	if ingress.Spec.IngressClassName != nil {
		metadata["ingress_class"] = *ingress.Spec.IngressClassName
	}

	if len(ingress.Labels) > 0 {
		var labelPairs []string
		for key, value := range ingress.Labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", key, value))
		}
		metadata["labels"] = strings.Join(labelPairs, ",")
	}

	if len(ingress.Annotations) > 0 {
		schema.AddMetadataInt(metadata, "annotations_count", len(ingress.Annotations))
		if certManager, ok := ingress.Annotations["cert-manager.io/cluster-issuer"]; ok {
			metadata["cert_issuer"] = certManager
		}
		if nginxClass, ok := ingress.Annotations["kubernetes.io/ingress.class"]; ok {
			metadata["ingress_class_annotation"] = nginxClass
		}
	}

	if len(ingress.Spec.TLS) > 0 {
		var tlsHosts []string
		var tlsSecrets []string
		for _, tls := range ingress.Spec.TLS {
			tlsHosts = append(tlsHosts, tls.Hosts...)
			if tls.SecretName != "" {
				tlsSecrets = append(tlsSecrets, tls.SecretName)
			}
		}
		if len(tlsHosts) > 0 {
			metadata["tls_hosts"] = strings.Join(tlsHosts, ",")
		}
		if len(tlsSecrets) > 0 {
			metadata["tls_secrets"] = strings.Join(tlsSecrets, ",")
		}
		schema.AddMetadataInt(metadata, "tls_count", len(ingress.Spec.TLS))
	}
	schema.AddMetadataInt(metadata, "rules_count", len(ingress.Spec.Rules))

	var backendServices []string
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					backendService := fmt.Sprintf("%s:%v", path.Backend.Service.Name, path.Backend.Service.Port.Number)
					if path.Backend.Service.Port.Name != "" {
						backendService = fmt.Sprintf("%s:%s", path.Backend.Service.Name, path.Backend.Service.Port.Name)
					}
					backendServices = append(backendServices, backendService)
				}
			}
		}
	}
	if len(backendServices) > 0 {
		// Deduplicate backend services
		uniqueBackends := make(map[string]struct{})
		for _, backend := range backendServices {
			uniqueBackends[backend] = struct{}{}
		}
		var uniqueBackendsList []string
		for backend := range uniqueBackends {
			uniqueBackendsList = append(uniqueBackendsList, backend)
		}
		metadata["backend_services"] = strings.Join(uniqueBackendsList, ",")
	}

	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		defaultBackend := fmt.Sprintf("%s:%v", ingress.Spec.DefaultBackend.Service.Name, ingress.Spec.DefaultBackend.Service.Port.Number)
		if ingress.Spec.DefaultBackend.Service.Port.Name != "" {
			defaultBackend = fmt.Sprintf("%s:%s", ingress.Spec.DefaultBackend.Service.Name, ingress.Spec.DefaultBackend.Service.Port.Name)
		}
		metadata["default_backend"] = defaultBackend
	}
	if len(ingress.Status.LoadBalancer.Ingress) > 0 {
		schema.AddMetadataInt(metadata, "load_balancer_count", len(ingress.Status.LoadBalancer.Ingress))
	}
	if managedBy, ok := ingress.Labels["app.kubernetes.io/managed-by"]; ok {
		metadata["managed_by"] = managedBy
	}
	if instance, ok := ingress.Labels["app.kubernetes.io/instance"]; ok {
		metadata["instance"] = instance
	}
	if version, ok := ingress.Labels["app.kubernetes.io/version"]; ok {
		metadata["version"] = version
	}

	return metadata
}
