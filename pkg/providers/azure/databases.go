package azure

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cosmos/armcosmos/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/mysql/armmysqlflexibleservers/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/postgresql/armpostgresqlflexibleservers/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/redis/armredis/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/sql/armsql"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// databaseProvider is a provider for Azure managed database endpoints
// (Azure Cache for Redis, MySQL and PostgreSQL flexible servers, SQL servers, Cosmos DB)
type databaseProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
	// clientOptions is nil outside tests, which point it at a fake ARM endpoint.
	clientOptions *arm.ClientOptions
}

type databaseFetcher struct {
	service string
	fetch   func(context.Context) (*schema.Resources, error)
}

// fetchers pairs each database service name with its lister
func (dp *databaseProvider) fetchers() []databaseFetcher {
	return []databaseFetcher{
		{"redis", dp.redis},
		{"mysql", dp.mysql},
		{"postgresql", dp.postgresql},
		{"sql", dp.sql},
		{"cosmosdb", dp.cosmosdb},
	}
}

// add appends a hostname. Public network access is recorded in metadata
// rather than skipping the host, since a private one still resolves through
// its privatelink CNAME and belongs in the inventory.
func (dp *databaseProvider) add(list *schema.Resources, service, host string, metadata map[string]string) {
	if host == "" {
		return
	}
	list.Append(&schema.Resource{
		Provider: providerName,
		ID:       dp.id,
		DNSName:  host,
		Service:  service,
		Metadata: metadata,
	})
}

func (dp *databaseProvider) redis(ctx context.Context) (*schema.Resources, error) {
	client, err := armredis.NewClient(dp.SubscriptionID, dp.Credential, dp.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}

	list := schema.NewResources()
	pager := client.NewListBySubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list redis caches: %w", err)
		}
		for _, cache := range page.Value {
			if cache == nil || cache.Properties == nil || cache.Properties.HostName == nil {
				continue
			}
			var metadata map[string]string
			if dp.extendedMetadata {
				props := cache.Properties
				metadata = dp.metadata(cache.ID, cache.Name, cache.Location, cache.Tags)
				addEnumMetadata(metadata, "public_network_access", props.PublicNetworkAccess)
				addEnumMetadata(metadata, "provisioning_state", props.ProvisioningState)
				schema.AddMetadata(metadata, "redis_version", props.RedisVersion)
				addInt32Metadata(metadata, "port", props.Port)
				addInt32Metadata(metadata, "ssl_port", props.SSLPort)
				if props.EnableNonSSLPort != nil {
					metadata["non_ssl_port_enabled"] = fmt.Sprintf("%v", *props.EnableNonSSLPort)
				}
			}
			dp.add(list, "redis", *cache.Properties.HostName, metadata)
		}
	}
	return list, nil
}

func (dp *databaseProvider) mysql(ctx context.Context) (*schema.Resources, error) {
	client, err := armmysqlflexibleservers.NewServersClient(dp.SubscriptionID, dp.Credential, dp.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create mysql servers client: %w", err)
	}

	list := schema.NewResources()
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list mysql flexible servers: %w", err)
		}
		for _, server := range page.Value {
			if server == nil || server.Properties == nil || server.Properties.FullyQualifiedDomainName == nil {
				continue
			}
			var metadata map[string]string
			if dp.extendedMetadata {
				props := server.Properties
				metadata = dp.metadata(server.ID, server.Name, server.Location, server.Tags)
				if props.Network != nil {
					addEnumMetadata(metadata, "public_network_access", props.Network.PublicNetworkAccess)
				}
				addEnumMetadata(metadata, "version", props.Version)
				addEnumMetadata(metadata, "state", props.State)
				schema.AddMetadata(metadata, "administrator_login", props.AdministratorLogin)
				if server.SKU != nil {
					schema.AddMetadata(metadata, "sku_name", server.SKU.Name)
				}
			}
			dp.add(list, "mysql", *server.Properties.FullyQualifiedDomainName, metadata)
		}
	}
	return list, nil
}

func (dp *databaseProvider) postgresql(ctx context.Context) (*schema.Resources, error) {
	client, err := armpostgresqlflexibleservers.NewServersClient(dp.SubscriptionID, dp.Credential, dp.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgresql servers client: %w", err)
	}

	list := schema.NewResources()
	pager := client.NewListBySubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list postgresql flexible servers: %w", err)
		}
		for _, server := range page.Value {
			if server == nil || server.Properties == nil || server.Properties.FullyQualifiedDomainName == nil {
				continue
			}
			var metadata map[string]string
			if dp.extendedMetadata {
				props := server.Properties
				metadata = dp.metadata(server.ID, server.Name, server.Location, server.Tags)
				if props.Network != nil {
					addEnumMetadata(metadata, "public_network_access", props.Network.PublicNetworkAccess)
				}
				addEnumMetadata(metadata, "version", props.Version)
				addEnumMetadata(metadata, "state", props.State)
				schema.AddMetadata(metadata, "administrator_login", props.AdministratorLogin)
				if server.SKU != nil {
					schema.AddMetadata(metadata, "sku_name", server.SKU.Name)
				}
			}
			dp.add(list, "postgresql", *server.Properties.FullyQualifiedDomainName, metadata)
		}
	}
	return list, nil
}

func (dp *databaseProvider) sql(ctx context.Context) (*schema.Resources, error) {
	client, err := armsql.NewServersClient(dp.SubscriptionID, dp.Credential, dp.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create sql servers client: %w", err)
	}

	list := schema.NewResources()
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list sql servers: %w", err)
		}
		for _, server := range page.Value {
			if server == nil || server.Properties == nil || server.Properties.FullyQualifiedDomainName == nil {
				continue
			}
			var metadata map[string]string
			if dp.extendedMetadata {
				props := server.Properties
				metadata = dp.metadata(server.ID, server.Name, server.Location, server.Tags)
				addEnumMetadata(metadata, "public_network_access", props.PublicNetworkAccess)
				schema.AddMetadata(metadata, "version", props.Version)
				schema.AddMetadata(metadata, "state", props.State)
				schema.AddMetadata(metadata, "minimal_tls_version", props.MinimalTLSVersion)
				schema.AddMetadata(metadata, "administrator_login", props.AdministratorLogin)
			}
			dp.add(list, "sql", *server.Properties.FullyQualifiedDomainName, metadata)
		}
	}
	return list, nil
}

func (dp *databaseProvider) cosmosdb(ctx context.Context) (*schema.Resources, error) {
	client, err := armcosmos.NewDatabaseAccountsClient(dp.SubscriptionID, dp.Credential, dp.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create cosmos db accounts client: %w", err)
	}

	list := schema.NewResources()
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list cosmos db accounts: %w", err)
		}
		for _, account := range page.Value {
			if account == nil || account.Properties == nil {
				continue
			}
			props := account.Properties
			var metadata map[string]string
			if dp.extendedMetadata {
				metadata = dp.metadata(account.ID, account.Name, account.Location, account.Tags)
				addEnumMetadata(metadata, "public_network_access", props.PublicNetworkAccess)
				addEnumMetadata(metadata, "kind", account.Kind)
				schema.AddMetadata(metadata, "provisioning_state", props.ProvisioningState)
				addEnumMetadata(metadata, "minimal_tls_version", props.MinimalTLSVersion)
				if props.DisableLocalAuth != nil {
					metadata["local_auth_disabled"] = fmt.Sprintf("%v", *props.DisableLocalAuth)
				}
			}
			// MongoDB 3.6 and later serve the wire protocol on a sibling host the API
			// does not return. Version 3.2, and accounts that omit the version, use
			// the documents host. Deriving the sibling keeps the sovereign cloud suffix.
			mongoWire := account.Kind != nil && *account.Kind == armcosmos.DatabaseAccountKindMongoDB &&
				props.APIProperties != nil && props.APIProperties.ServerVersion != nil &&
				*props.APIProperties.ServerVersion != armcosmos.ServerVersionThree2
			addEndpoint := func(endpoint *string) {
				if endpoint == nil {
					return
				}
				host := extractDNSFromURL(*endpoint)
				dp.add(list, "cosmosdb", host, metadata)
				if mongoWire {
					dp.add(list, "cosmosdb", strings.Replace(host, ".documents.", ".mongo.cosmos.", 1), metadata)
				}
			}
			addEndpoint(props.DocumentEndpoint)
			// Multi-region accounts also serve each region on its own hostname.
			for _, location := range slices.Concat(props.WriteLocations, props.ReadLocations) {
				if location != nil {
					addEndpoint(location.DocumentEndpoint)
				}
			}
		}
	}
	return list, nil
}

func (dp *databaseProvider) metadata(resourceID, name, location *string, tags map[string]*string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "name", name)
	schema.AddMetadata(metadata, "resource_id", resourceID)
	metadata["subscription_id"] = dp.SubscriptionID
	metadata["owner_id"] = dp.SubscriptionID
	schema.AddMetadata(metadata, "location", location)
	if resourceID != nil {
		if _, resourceGroup := parseAzureResourceID(*resourceID); resourceGroup != "" {
			metadata["resource_group"] = resourceGroup
		}
	}
	if tagString := buildAzureTagString(tags); tagString != "" {
		metadata["tags"] = tagString
	}
	return metadata
}

func addEnumMetadata[T ~string](metadata map[string]string, key string, value *T) {
	if value != nil && *value != "" {
		metadata[key] = string(*value)
	}
}

func addInt32Metadata(metadata map[string]string, key string, value *int32) {
	if value != nil {
		metadata[key] = fmt.Sprintf("%d", *value)
	}
}
