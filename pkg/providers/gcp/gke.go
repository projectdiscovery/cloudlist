package gcp

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/providers/k8s"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	container "google.golang.org/api/container/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// gkeProvider is a provider for aws Route53 API
type gkeProvider struct {
	id       string
	svc      *container.Service
	projects []string
}

func (d *gkeProvider) name() string {
	return "gke"
}

// GetResource returns all the resources in the store for a provider.
func (d *gkeProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		kubeConfig, err := d.getK8sClusterConfigs(ctx, project)
		if err != nil {
			return nil, err
		}
		// Just list all the namespaces found in the project to test the API.
		for clusterName := range kubeConfig.Clusters {
			cfg, err := clientcmd.NewNonInteractiveClientConfig(*kubeConfig, clusterName, &clientcmd.ConfigOverrides{CurrentContext: clusterName}, nil).ClientConfig()
			if err != nil {
				return nil, fmt.Errorf("failed to create Kubernetes configuration cluster=%s: %w", clusterName, err)
			}

			k8sClient, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create Kubernetes client cluster=%s: %w", clusterName, err)
			}
			timeoutSeconds := int64(10)
			ingress, err := k8sClient.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{
				TimeoutSeconds: &timeoutSeconds,
			})
			if err != nil {
				return nil, errors.Wrap(err, "could not list kubernetes ingress")
			}
			k8sIngressProvider := k8s.NewK8sIngressProvider(d.id, ingress)
			ingressHosts, _ := k8sIngressProvider.GetResource(ctx)
			for _, ingressHost := range ingressHosts.Items {
				ingressHost.Service = d.name()
			}
			list.Merge(ingressHosts)
		}
	}
	return list, nil
}

func (d *gkeProvider) getK8sClusterConfigs(ctx context.Context, projectId string) (*api.Config, error) {
	// Basic config structure
	ret := api.Config{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters:   map[string]*api.Cluster{},  // Clusters is a map of referencable names to cluster configs
		AuthInfos:  map[string]*api.AuthInfo{}, // AuthInfos is a map of referencable names to user configs
		Contexts:   map[string]*api.Context{},  // Contexts is a map of referencable names to context configs
	}

	// Ask Google for a list of all kube clusters in the given project.
	resp, err := d.svc.Projects.Zones.Clusters.List(projectId, "-").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("clusters list project=%s: %w", projectId, err)
	}

	for _, f := range resp.Clusters {
		name := fmt.Sprintf("gke_%s_%s_%s", projectId, f.Zone, f.Name)
		cert, err := base64.StdEncoding.DecodeString(f.MasterAuth.ClusterCaCertificate)
		if err != nil {
			return nil, fmt.Errorf("invalid certificate cluster=%s cert=%s: %w", name, f.MasterAuth.ClusterCaCertificate, err)
		}
		// example: gke_my-project_us-central1-b_cluster-1 => https://XX.XX.XX.XX
		ret.Clusters[name] = &api.Cluster{
			CertificateAuthorityData: cert,
			Server:                   "https://" + f.Endpoint,
		}
		// Just reuse the context name as an auth name.
		ret.Contexts[name] = &api.Context{
			Cluster:  name,
			AuthInfo: name,
		}
		// GCP specific configation;
		ret.AuthInfos[name] = &api.AuthInfo{
			AuthProvider: &api.AuthProviderConfig{Name: googleAuthPlugin},
		}
	}
	return &ret, nil
}
