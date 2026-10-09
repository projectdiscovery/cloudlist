package scaleway

import (
	"context"
	"net/url"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	container "github.com/scaleway/scaleway-sdk-go/api/container/v1beta1"
	function "github.com/scaleway/scaleway-sdk-go/api/function/v1beta1"
	k8s "github.com/scaleway/scaleway-sdk-go/api/k8s/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// containerProvider lists endpoints of Serverless Containers, Serverless
// Functions and Kapsule clusters.
type containerProvider struct {
	id           string
	containerAPI *container.API
	functionAPI  *function.API
	k8sAPI       *k8s.API
}

// GetContainers returns Serverless Containers domain names.
// Listing goes through namespaces because this SDK version always sends
// namespace_id on ListContainers.
func (d *containerProvider) GetContainers(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var listingErr error

	err := forEachLocality(d.containerAPI.Regions(), func(region scw.Region) error {
		namespaces, err := d.containerAPI.ListNamespaces(&container.ListNamespacesRequest{Region: region}, scw.WithAllPages(), scw.WithContext(ctx))
		if err != nil {
			return err
		}
		for _, namespace := range namespaces.Namespaces {
			resp, err := d.containerAPI.ListContainers(&container.ListContainersRequest{Region: region, NamespaceID: namespace.ID}, scw.WithAllPages(), scw.WithContext(ctx))
			if err != nil {
				listingErr = err
				continue
			}
			for _, c := range resp.Containers {
				list.Append(d.dnsResource(c.DomainName, "container"))
			}
		}
		return nil
	})
	if listingErr != nil {
		err = listingErr
	}
	if err != nil && len(list.Items) == 0 {
		return nil, err
	}
	return list, err
}

// GetFunctions returns Serverless Functions domain names.
func (d *containerProvider) GetFunctions(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var listingErr error

	err := forEachLocality(d.functionAPI.Regions(), func(region scw.Region) error {
		namespaces, err := d.functionAPI.ListNamespaces(&function.ListNamespacesRequest{Region: region}, scw.WithAllPages(), scw.WithContext(ctx))
		if err != nil {
			return err
		}
		for _, namespace := range namespaces.Namespaces {
			resp, err := d.functionAPI.ListFunctions(&function.ListFunctionsRequest{Region: region, NamespaceID: namespace.ID}, scw.WithAllPages(), scw.WithContext(ctx))
			if err != nil {
				listingErr = err
				continue
			}
			for _, f := range resp.Functions {
				list.Append(d.dnsResource(f.DomainName, "function"))
			}
		}
		return nil
	})
	if listingErr != nil {
		err = listingErr
	}
	if err != nil && len(list.Items) == 0 {
		return nil, err
	}
	return list, err
}

// GetKapsuleClusters returns Kapsule cluster API server hostnames.
func (d *containerProvider) GetKapsuleClusters(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	err := forEachLocality(d.k8sAPI.Regions(), func(region scw.Region) error {
		resp, err := d.k8sAPI.ListClusters(&k8s.ListClustersRequest{Region: region}, scw.WithAllPages(), scw.WithContext(ctx))
		if err != nil {
			return err
		}
		for _, cluster := range resp.Clusters {
			// ClusterURL is a full URL such as https://<id>.api.k8s.fr-par.scw.cloud:6443
			if u, err := url.Parse(cluster.ClusterURL); err == nil {
				list.Append(d.dnsResource(u.Hostname(), "kapsule"))
			}
		}
		return nil
	})
	if err != nil && len(list.Items) == 0 {
		return nil, err
	}
	return list, err
}

func (d *containerProvider) dnsResource(hostname, service string) *schema.Resource {
	return &schema.Resource{
		Provider: providerName,
		ID:       d.id,
		DNSName:  hostname,
		Public:   true,
		Service:  service,
	}
}
