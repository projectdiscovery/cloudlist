package gcp

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	container "google.golang.org/api/container/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/rest"
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
		req := d.svc.Projects.Locations.Clusters.List(fmt.Sprintf("projects/%s/locations/-", project))
		resp, err := req.Do()
		if err != nil {
			return nil, fmt.Errorf("could not list GKE clusters for project %s: %s", project, ExtractGoogleErrorReason(err))
		}
		if resp.Clusters == nil {
			continue
		}
		for _, cluster := range resp.Clusters {
			config, err := BuildClientConfig(cluster)
			if err != nil {
				continue
			}
			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				continue
			}
			nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}
			for range nodes.Items {
				// Example logic: we just know the cluster might expose nodes publicly.
				list.Append(&schema.Resource{
					Public:   true,
					ID:       d.id,
					Provider: providerName,
					Service:  d.name(),
				})
			}
		}
	}
	return list, nil
}

// BuildClientConfig returns a client config for kubernetes
func BuildClientConfig(cluster *container.Cluster) (*rest.Config, error) {
	if len(cluster.MasterAuth.ClusterCaCertificate) == 0 {
		return nil, errors.New("error creating k8s client: no CA certificate")
	}

	sDec, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, err
	}

	userConfig := api.Config{
		AuthInfos: map[string]*api.AuthInfo{
			"user": {
				AuthProvider: &api.AuthProviderConfig{
					Name: googleAuthPlugin,
				},
			},
		},
		Clusters: map[string]*api.Cluster{
			"cluster": {
				CertificateAuthorityData: sDec,
				Server:                   fmt.Sprintf("https://%s", cluster.Endpoint),
			},
		},
		Contexts: map[string]*api.Context{
			"context": {
				AuthInfo: "user",
				Cluster:  "cluster",
			},
		},
		CurrentContext: "context",
	}

	return clientcmd.NewNonInteractiveClientConfig(userConfig, "context", nil, nil).ClientConfig()
}
