package azure

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/servicefabric/armservicefabric"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/servicefabricmanagedclusters/armservicefabricmanagedclusters"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// serviceFabricProvider is a provider for classic and managed Azure Service Fabric clusters
type serviceFabricProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
	clientOptions    *arm.ClientOptions
}

// name returns the name of the provider
func (sp *serviceFabricProvider) name() string {
	return "servicefabric"
}

// GetResource returns the endpoints of classic and managed Service Fabric clusters
func (sp *serviceFabricProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	// The two cluster kinds live in separate resource providers, so one being
	// unregistered or forbidden must not hide the other.
	classic, classicErr := sp.fetchClusters(ctx)
	for _, cluster := range classic {
		if cluster.Properties == nil || cluster.Properties.ManagementEndpoint == nil {
			continue
		}
		endpoint, err := url.Parse(*cluster.Properties.ManagementEndpoint)
		if err != nil || endpoint.Hostname() == "" {
			continue
		}

		var metadata map[string]string
		if sp.extendedMetadata {
			metadata = sp.getClusterMetadata(cluster)
		}

		list.Append(&schema.Resource{
			Provider: providerName,
			ID:       sp.id,
			DNSName:  endpoint.Hostname(),
			Service:  sp.name(),
			Metadata: metadata,
		})
	}

	managed, managedErr := sp.fetchManagedClusters(ctx)
	for _, cluster := range managed {
		if cluster.Properties == nil {
			continue
		}
		props := cluster.Properties

		var metadata map[string]string
		if sp.extendedMetadata {
			metadata = sp.getManagedClusterMetadata(cluster)
		}

		list.Append(&schema.Resource{
			Provider:   providerName,
			ID:         sp.id,
			DNSName:    stringValue(props.Fqdn),
			PublicIPv4: stringValue(props.IPv4Address),
			PublicIPv6: stringValue(props.IPv6Address),
			Service:    sp.name(),
			Metadata:   metadata,
		})
	}

	if len(list.Items) == 0 && classicErr != nil && managedErr != nil {
		return nil, fmt.Errorf("%w; %w", classicErr, managedErr)
	}
	for _, err := range []error{classicErr, managedErr} {
		if err != nil {
			gologger.Warning().Msgf("Error listing Service Fabric clusters for subscription %s: %s", sp.SubscriptionID, err)
		}
	}
	return list, nil
}

func (sp *serviceFabricProvider) fetchClusters(ctx context.Context) ([]*armservicefabric.Cluster, error) {
	client, err := armservicefabric.NewClustersClient(sp.SubscriptionID, sp.Credential, sp.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create Service Fabric clusters client: %w", err)
	}

	// The classic API returns every cluster in the subscription in one response.
	resp, err := client.List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list Service Fabric clusters: %w", err)
	}
	return resp.Value, nil
}

func (sp *serviceFabricProvider) fetchManagedClusters(ctx context.Context) ([]*armservicefabricmanagedclusters.ManagedCluster, error) {
	client, err := armservicefabricmanagedclusters.NewManagedClustersClient(sp.SubscriptionID, sp.Credential, sp.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create Service Fabric managed clusters client: %w", err)
	}

	var clusters []*armservicefabricmanagedclusters.ManagedCluster
	pager := client.NewListBySubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			// A later page can fail after earlier clusters were collected.
			return clusters, fmt.Errorf("failed to list Service Fabric managed clusters: %w", err)
		}
		clusters = append(clusters, page.Value...)
	}
	return clusters, nil
}

func (sp *serviceFabricProvider) getClusterMetadata(cluster *armservicefabric.Cluster) map[string]string {
	metadata := sp.baseMetadata(cluster.Name, cluster.ID, cluster.Location, cluster.Type, cluster.Tags)
	metadata["cluster_kind"] = "classic"

	props := cluster.Properties
	schema.AddMetadata(metadata, "management_endpoint", props.ManagementEndpoint)
	schema.AddMetadata(metadata, "cluster_code_version", props.ClusterCodeVersion)
	schema.AddMetadata(metadata, "vm_image", props.VMImage)
	if props.ClusterState != nil {
		metadata["cluster_state"] = string(*props.ClusterState)
	}
	if props.ProvisioningState != nil {
		metadata["provisioning_state"] = string(*props.ProvisioningState)
	}
	if props.ReliabilityLevel != nil {
		metadata["reliability_level"] = string(*props.ReliabilityLevel)
	}
	return metadata
}

func (sp *serviceFabricProvider) getManagedClusterMetadata(cluster *armservicefabricmanagedclusters.ManagedCluster) map[string]string {
	metadata := sp.baseMetadata(cluster.Name, cluster.ID, cluster.Location, cluster.Type, cluster.Tags)
	metadata["cluster_kind"] = "managed"

	props := cluster.Properties
	schema.AddMetadata(metadata, "dns_name", props.DNSName)
	schema.AddMetadata(metadata, "cluster_code_version", props.ClusterCodeVersion)
	if props.ClientConnectionPort != nil {
		metadata["client_connection_port"] = fmt.Sprintf("%d", *props.ClientConnectionPort)
	}
	if props.HTTPGatewayConnectionPort != nil {
		metadata["http_gateway_connection_port"] = fmt.Sprintf("%d", *props.HTTPGatewayConnectionPort)
	}
	if props.ClusterState != nil {
		metadata["cluster_state"] = string(*props.ClusterState)
	}
	if props.ProvisioningState != nil {
		metadata["provisioning_state"] = string(*props.ProvisioningState)
	}
	if cluster.SKU != nil && cluster.SKU.Name != nil {
		metadata["sku"] = string(*cluster.SKU.Name)
	}
	return metadata
}

func (sp *serviceFabricProvider) baseMetadata(name, id, location, resourceType *string, tags map[string]*string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "cluster_name", name)
	schema.AddMetadata(metadata, "cluster_id", id)
	metadata["subscription_id"] = sp.SubscriptionID
	metadata["owner_id"] = sp.SubscriptionID
	schema.AddMetadata(metadata, "location", location)
	schema.AddMetadata(metadata, "type", resourceType)

	if id != nil {
		if _, resourceGroup := parseAzureResourceID(*id); resourceGroup != "" {
			metadata["resource_group"] = resourceGroup
		}
	}
	if tagString := buildAzureTagString(tags); tagString != "" {
		metadata["tags"] = tagString
	}
	return metadata
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
