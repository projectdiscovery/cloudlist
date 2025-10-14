package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// storageProvider is a provider for Azure Storage Accounts
type storageProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (sp *storageProvider) name() string {
	return "storage"
}

// GetResource returns all the Storage Account endpoints for a provider.
func (sp *storageProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	accounts, err := sp.fetchStorageAccounts(ctx)
	if err != nil {
		return nil, err
	}

	for _, account := range accounts {
		if account.Properties == nil {
			continue
		}

		// Extract primary endpoints
		if account.Properties.PrimaryEndpoints != nil {
			endpoints := account.Properties.PrimaryEndpoints

			var metadata map[string]string
			if sp.extendedMetadata {
				metadata = sp.getStorageMetadata(account)
			}

			// Blob endpoint: *.blob.core.windows.net
			if endpoints.Blob != nil && *endpoints.Blob != "" {
				if blobHost := extractHostFromURL(*endpoints.Blob); blobHost != "" {
					resource := &schema.Resource{
						Provider: providerName,
						ID:       sp.id,
						DNSName:  blobHost,
						Service:  sp.name(),
					}
					if metadata != nil {
						resource.Metadata = copyMetadata(metadata)
					}
					list.Append(resource)
				}
			}

			// File endpoint: *.file.core.windows.net
			if endpoints.File != nil && *endpoints.File != "" {
				if fileHost := extractHostFromURL(*endpoints.File); fileHost != "" {
					resource := &schema.Resource{
						Provider: providerName,
						ID:       sp.id,
						DNSName:  fileHost,
						Service:  sp.name(),
					}
					if metadata != nil {
						resource.Metadata = copyMetadata(metadata)
					}
					list.Append(resource)
				}
			}

			// Table endpoint: *.table.core.windows.net
			if endpoints.Table != nil && *endpoints.Table != "" {
				if tableHost := extractHostFromURL(*endpoints.Table); tableHost != "" {
					resource := &schema.Resource{
						Provider: providerName,
						ID:       sp.id,
						DNSName:  tableHost,
						Service:  sp.name(),
					}
					if metadata != nil {
						resource.Metadata = copyMetadata(metadata)
					}
					list.Append(resource)
				}
			}

			// Queue endpoint: *.queue.core.windows.net
			if endpoints.Queue != nil && *endpoints.Queue != "" {
				if queueHost := extractHostFromURL(*endpoints.Queue); queueHost != "" {
					resource := &schema.Resource{
						Provider: providerName,
						ID:       sp.id,
						DNSName:  queueHost,
						Service:  sp.name(),
					}
					if metadata != nil {
						resource.Metadata = copyMetadata(metadata)
					}
					list.Append(resource)
				}
			}

			// DFS endpoint (Data Lake): *.dfs.core.windows.net
			if endpoints.Dfs != nil && *endpoints.Dfs != "" {
				if dfsHost := extractHostFromURL(*endpoints.Dfs); dfsHost != "" {
					resource := &schema.Resource{
						Provider: providerName,
						ID:       sp.id,
						DNSName:  dfsHost,
						Service:  sp.name(),
					}
					if metadata != nil {
						resource.Metadata = copyMetadata(metadata)
					}
					list.Append(resource)
				}
			}

			// Web endpoint: *.web.core.windows.net
			if endpoints.Web != nil && *endpoints.Web != "" {
				if webHost := extractHostFromURL(*endpoints.Web); webHost != "" {
					resource := &schema.Resource{
						Provider: providerName,
						ID:       sp.id,
						DNSName:  webHost,
						Service:  sp.name(),
					}
					if metadata != nil {
						resource.Metadata = copyMetadata(metadata)
					}
					list.Append(resource)
				}
			}
		}

		// Check for custom domains
		if account.Properties.CustomDomain != nil && account.Properties.CustomDomain.Name != nil && *account.Properties.CustomDomain.Name != "" {
			var metadata map[string]string
			if sp.extendedMetadata {
				metadata = sp.getStorageMetadata(account)
				if metadata != nil {
					metadata["custom_domain"] = "true"
				}
			}

			resource := &schema.Resource{
				Provider: providerName,
				ID:       sp.id,
				DNSName:  *account.Properties.CustomDomain.Name,
				Service:  sp.name(),
				Metadata: metadata,
			}
			list.Append(resource)
		}

		// Check for Private Endpoints to extract private IPs
		if account.Properties.PrivateEndpointConnections != nil && len(account.Properties.PrivateEndpointConnections) > 0 {
			privateIPs := sp.extractPrivateEndpointIPs(ctx, account)
			for _, privateIP := range privateIPs {
				var metadata map[string]string
				if sp.extendedMetadata {
					metadata = sp.getStorageMetadata(account)
					if metadata != nil {
						metadata["connection_type"] = "private_endpoint"
					}
				}

				resource := &schema.Resource{
					Provider:    providerName,
					ID:          sp.id,
					PrivateIpv4: privateIP,
					Service:     sp.name(),
					Metadata:    metadata,
				}
				list.Append(resource)
			}
		}
	}

	return list, nil
}

func (sp *storageProvider) fetchStorageAccounts(ctx context.Context) ([]*armstorage.Account, error) {
	var accounts []*armstorage.Account

	// Create storage accounts client
	client, err := armstorage.NewAccountsClient(sp.SubscriptionID, sp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage accounts client: %w", err)
	}

	// Use pager pattern
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list storage accounts: %w", err)
		}

		accounts = append(accounts, page.Value...)
	}

	return accounts, nil
}

func (sp *storageProvider) extractPrivateEndpointIPs(ctx context.Context, account *armstorage.Account) []string {
	var privateIPs []string

	if account.Properties == nil || account.Properties.PrivateEndpointConnections == nil {
		return privateIPs
	}

	for _, peConn := range account.Properties.PrivateEndpointConnections {
		if peConn.Properties == nil || peConn.Properties.PrivateEndpoint == nil || peConn.Properties.PrivateEndpoint.ID == nil {
			continue
		}

		// Parse Private Endpoint resource ID
		peName, peRG := parseAzureResourceID(*peConn.Properties.PrivateEndpoint.ID)
		if peName == "" || peRG == "" {
			continue
		}

		// Fetch Private Endpoint details
		peClient, err := armnetwork.NewPrivateEndpointsClient(sp.SubscriptionID, sp.Credential, nil)
		if err != nil {
			gologger.Warning().Msgf("failed to create private endpoints client: %s", err)
			continue
		}

		pe, err := peClient.Get(ctx, peRG, peName, nil)
		if err != nil {
			gologger.Warning().Msgf("failed to get private endpoint %s: %s", peName, err)
			continue
		}

		// Extract private IPs from network interfaces
		if pe.Properties != nil && pe.Properties.NetworkInterfaces != nil {
			for _, nic := range pe.Properties.NetworkInterfaces {
				if nic.ID == nil {
					continue
				}

				nicName, nicRG := parseAzureResourceID(*nic.ID)
				if nicName == "" || nicRG == "" {
					continue
				}

				nicClient, err := armnetwork.NewInterfacesClient(sp.SubscriptionID, sp.Credential, nil)
				if err != nil {
					gologger.Warning().Msgf("failed to create network interfaces client: %s", err)
					continue
				}

				nicRes, err := nicClient.Get(ctx, nicRG, nicName, nil)
				if err != nil {
					gologger.Warning().Msgf("failed to get network interface %s: %s", nicName, err)
					continue
				}

				if nicRes.Properties != nil && nicRes.Properties.IPConfigurations != nil {
					for _, ipConfig := range nicRes.Properties.IPConfigurations {
						if ipConfig.Properties != nil && ipConfig.Properties.PrivateIPAddress != nil {
							privateIPs = append(privateIPs, *ipConfig.Properties.PrivateIPAddress)
						}
					}
				}
			}
		}
	}

	return privateIPs
}

func (sp *storageProvider) getStorageMetadata(account *armstorage.Account) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "storage_account_name", account.Name)
	schema.AddMetadata(metadata, "storage_account_id", account.ID)
	metadata["subscription_id"] = sp.SubscriptionID
	metadata["owner_id"] = sp.SubscriptionID
	schema.AddMetadata(metadata, "location", account.Location)
	schema.AddMetadata(metadata, "type", account.Type)

	// Parse resource group from ID
	if account.ID != nil {
		parts := strings.Split(*account.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	if account.SKU != nil {
		if account.SKU.Name != nil {
			skuName := string(*account.SKU.Name)
			metadata["sku_name"] = skuName
		}
		if account.SKU.Tier != nil {
			skuTier := string(*account.SKU.Tier)
			metadata["sku_tier"] = skuTier
		}
	}

	if account.Kind != nil {
		kind := string(*account.Kind)
		metadata["storage_kind"] = kind
	}

	if account.Properties != nil {
		props := account.Properties

		if props.ProvisioningState != nil {
			provisioningState := string(*props.ProvisioningState)
			metadata["provisioning_state"] = provisioningState
		}

		if props.AccessTier != nil {
			accessTier := string(*props.AccessTier)
			metadata["access_tier"] = accessTier
		}

		if props.EnableHTTPSTrafficOnly != nil {
			metadata["https_only"] = fmt.Sprintf("%v", *props.EnableHTTPSTrafficOnly)
		}

		if props.AllowBlobPublicAccess != nil {
			metadata["allow_blob_public_access"] = fmt.Sprintf("%v", *props.AllowBlobPublicAccess)
		}

		if props.MinimumTLSVersion != nil {
			minTLS := string(*props.MinimumTLSVersion)
			metadata["minimum_tls_version"] = minTLS
		}

		if props.AllowSharedKeyAccess != nil {
			metadata["allow_shared_key_access"] = fmt.Sprintf("%v", *props.AllowSharedKeyAccess)
		}

		if props.IsHnsEnabled != nil {
			metadata["hierarchical_namespace_enabled"] = fmt.Sprintf("%v", *props.IsHnsEnabled)
		}

		if props.IsSftpEnabled != nil {
			metadata["sftp_enabled"] = fmt.Sprintf("%v", *props.IsSftpEnabled)
		}

		if props.IsLocalUserEnabled != nil {
			metadata["local_user_enabled"] = fmt.Sprintf("%v", *props.IsLocalUserEnabled)
		}

		if props.PublicNetworkAccess != nil {
			publicAccess := string(*props.PublicNetworkAccess)
			metadata["public_network_access"] = publicAccess
		}

		if props.NetworkRuleSet != nil {
			networkRules := props.NetworkRuleSet

			if networkRules.DefaultAction != nil {
				defaultAction := string(*networkRules.DefaultAction)
				metadata["network_acls_default_action"] = defaultAction
			}

			if networkRules.Bypass != nil {
				bypass := string(*networkRules.Bypass)
				metadata["network_acls_bypass"] = bypass
			}

			if networkRules.IPRules != nil && len(networkRules.IPRules) > 0 {
				var ipRules []string
				for _, rule := range networkRules.IPRules {
					if rule.IPAddressOrRange != nil {
						ipRules = append(ipRules, *rule.IPAddressOrRange)
					}
				}
				if len(ipRules) > 0 {
					metadata["network_acls_ip_rules"] = strings.Join(ipRules, ",")
				}
			}

			if networkRules.VirtualNetworkRules != nil && len(networkRules.VirtualNetworkRules) > 0 {
				metadata["network_acls_vnet_rules_count"] = fmt.Sprintf("%d", len(networkRules.VirtualNetworkRules))
			}
		}

		if props.Encryption != nil && props.Encryption.KeySource != nil {
			keySource := string(*props.Encryption.KeySource)
			metadata["encryption_key_source"] = keySource
		}

		if props.PrimaryEndpoints != nil {
			endpoints := props.PrimaryEndpoints
			schema.AddMetadata(metadata, "primary_blob_endpoint", endpoints.Blob)
			schema.AddMetadata(metadata, "primary_file_endpoint", endpoints.File)
			schema.AddMetadata(metadata, "primary_table_endpoint", endpoints.Table)
			schema.AddMetadata(metadata, "primary_queue_endpoint", endpoints.Queue)
			schema.AddMetadata(metadata, "primary_dfs_endpoint", endpoints.Dfs)
			schema.AddMetadata(metadata, "primary_web_endpoint", endpoints.Web)
		}

		if props.CustomDomain != nil {
			schema.AddMetadata(metadata, "custom_domain_name", props.CustomDomain.Name)
			if props.CustomDomain.UseSubDomainName != nil {
				metadata["custom_domain_use_subdomain"] = fmt.Sprintf("%v", *props.CustomDomain.UseSubDomainName)
			}
		}

		if props.CreationTime != nil {
			metadata["creation_time"] = props.CreationTime.Format("2006-01-02T15:04:05Z")
		}

		if props.PrivateEndpointConnections != nil && len(props.PrivateEndpointConnections) > 0 {
			metadata["private_endpoint_connections_count"] = fmt.Sprintf("%d", len(props.PrivateEndpointConnections))

			var peStates []string
			for _, peConn := range props.PrivateEndpointConnections {
				if peConn.Properties != nil && peConn.Properties.PrivateLinkServiceConnectionState != nil && peConn.Properties.PrivateLinkServiceConnectionState.Status != nil {
					status := string(*peConn.Properties.PrivateLinkServiceConnectionState.Status)
					peStates = append(peStates, status)
				}
			}
			if len(peStates) > 0 {
				metadata["private_endpoint_connection_states"] = strings.Join(peStates, ",")
			}
		}
	}

	if account.Identity != nil && account.Identity.Type != nil {
		identityType := string(*account.Identity.Type)
		metadata["identity_type"] = identityType

		if account.Identity.PrincipalID != nil {
			metadata["identity_principal_id"] = *account.Identity.PrincipalID
		}
	}

	if len(account.Tags) > 0 {
		if tagString := buildAzureTagString(account.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}

// extractHostFromURL extracts the hostname from a URL (removes https:// and trailing /)
func extractHostFromURL(urlStr string) string {
	// Remove protocol
	urlStr = strings.TrimPrefix(urlStr, "https://")
	urlStr = strings.TrimPrefix(urlStr, "http://")

	// Remove trailing slash and path
	if idx := strings.Index(urlStr, "/"); idx != -1 {
		urlStr = urlStr[:idx]
	}

	return strings.TrimSpace(urlStr)
}

// copyMetadata creates a copy of metadata map
func copyMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}

	copy := make(map[string]string)
	for k, v := range metadata {
		copy[k] = v
	}
	return copy
}
