package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// appServiceProvider is a provider for Azure App Service / Web Apps
type appServiceProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (asp *appServiceProvider) name() string {
	return "appservice"
}

// GetResource returns all the App Service Web Apps DNS endpoints for a provider.
func (asp *appServiceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	webApps, err := asp.fetchWebApps(ctx)
	if err != nil {
		return nil, err
	}

	for _, app := range webApps {
		if app.Properties == nil {
			continue
		}

		var metadata map[string]string
		if asp.extendedMetadata {
			metadata = asp.getAppServiceMetadata(app)
		}

		// Extract default hostname: <app-name>.azurewebsites.net
		if app.Properties.DefaultHostName != nil && *app.Properties.DefaultHostName != "" {
			resource := &schema.Resource{
				Provider: providerName,
				ID:       asp.id,
				DNSName:  *app.Properties.DefaultHostName,
				Service:  asp.name(),
				Metadata: metadata,
			}
			list.Append(resource)
		}

		// Extract custom domains if configured
		if app.Properties.HostNames != nil && len(app.Properties.HostNames) > 0 {
			for _, hostname := range app.Properties.HostNames {
				if hostname == nil || *hostname == "" {
					continue
				}

				// Skip if it's the default hostname (already added)
				if app.Properties.DefaultHostName != nil && *hostname == *app.Properties.DefaultHostName {
					continue
				}

				customResource := &schema.Resource{
					Provider: providerName,
					ID:       asp.id,
					DNSName:  *hostname,
					Service:  asp.name(),
				}

				// Copy metadata if available
				if metadata != nil {
					customResource.Metadata = make(map[string]string)
					for k, v := range metadata {
						customResource.Metadata[k] = v
					}
					customResource.Metadata["is_custom_domain"] = "true"
				}

				list.Append(customResource)
			}
		}

		// Extract deployment slot hostnames
		slots, err := asp.fetchDeploymentSlots(ctx, app)
		if err != nil {
			// Don't fail the entire operation, just log warning
			continue
		}

		for _, slot := range slots {
			if slot.Properties != nil && slot.Properties.DefaultHostName != nil && *slot.Properties.DefaultHostName != "" {
				var slotMetadata map[string]string
				if asp.extendedMetadata {
					slotMetadata = asp.getDeploymentSlotMetadata(slot, app)
				}

				slotResource := &schema.Resource{
					Provider: providerName,
					ID:       asp.id,
					DNSName:  *slot.Properties.DefaultHostName,
					Service:  asp.name(),
					Metadata: slotMetadata,
				}
				list.Append(slotResource)
			}
		}
	}

	return list, nil
}

func (asp *appServiceProvider) fetchWebApps(ctx context.Context) ([]*armappservice.Site, error) {
	// Track 2: Create web apps client directly
	client, err := armappservice.NewWebAppsClient(asp.SubscriptionID, asp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create web apps client: %w", err)
	}

	// Track 2: Use pager pattern
	var webApps []*armappservice.Site
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list web apps: %w", err)
		}

		webApps = append(webApps, page.Value...)
	}

	return webApps, nil
}

func (asp *appServiceProvider) fetchDeploymentSlots(ctx context.Context, app *armappservice.Site) ([]*armappservice.Site, error) {
	if app.Name == nil || app.ID == nil {
		return nil, nil
	}

	// Parse resource group from the app ID
	_, resourceGroup := parseAzureResourceID(*app.ID)
	if resourceGroup == "" {
		return nil, nil
	}

	// Track 2: Create web apps client
	client, err := armappservice.NewWebAppsClient(asp.SubscriptionID, asp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create web apps client: %w", err)
	}

	// Track 2: Use pager pattern for slots
	var slots []*armappservice.Site
	pager := client.NewListSlotsPager(resourceGroup, *app.Name, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			// Don't fail if slots listing fails (might not have slots)
			return nil, nil
		}

		slots = append(slots, page.Value...)
	}

	return slots, nil
}

func (asp *appServiceProvider) getAppServiceMetadata(app *armappservice.Site) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "app_name", app.Name)
	schema.AddMetadata(metadata, "app_id", app.ID)
	metadata["subscription_id"] = asp.SubscriptionID
	metadata["owner_id"] = asp.SubscriptionID
	schema.AddMetadata(metadata, "location", app.Location)
	schema.AddMetadata(metadata, "kind", app.Kind)
	schema.AddMetadata(metadata, "type", app.Type)

	// Parse resource group from ID
	if app.ID != nil {
		parts := strings.Split(*app.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	if app.Properties != nil {
		props := app.Properties

		schema.AddMetadata(metadata, "state", props.State)
		schema.AddMetadata(metadata, "default_hostname", props.DefaultHostName)
		schema.AddMetadata(metadata, "repository_site_name", props.RepositorySiteName)

		if props.Enabled != nil {
			metadata["enabled"] = fmt.Sprintf("%v", *props.Enabled)
		}

		if props.HTTPSOnly != nil {
			metadata["https_only"] = fmt.Sprintf("%v", *props.HTTPSOnly)
		}

		if props.ClientAffinityEnabled != nil {
			metadata["client_affinity_enabled"] = fmt.Sprintf("%v", *props.ClientAffinityEnabled)
		}

		schema.AddMetadata(metadata, "app_service_plan_id", props.ServerFarmID)
		schema.AddMetadata(metadata, "resource_group_name", props.ResourceGroup)
		schema.AddMetadata(metadata, "outbound_ip_addresses", props.OutboundIPAddresses)
		schema.AddMetadata(metadata, "possible_outbound_ip_addresses", props.PossibleOutboundIPAddresses)

		if props.EnabledHostNames != nil && len(props.EnabledHostNames) > 0 {
			var enabledHostnames []string
			for _, hostname := range props.EnabledHostNames {
				if hostname != nil {
					enabledHostnames = append(enabledHostnames, *hostname)
				}
			}
			if len(enabledHostnames) > 0 {
				metadata["enabled_hostnames"] = strings.Join(enabledHostnames, ",")
			}
		}

		if props.HostNames != nil && len(props.HostNames) > 0 {
			var customDomains []string
			for _, hostname := range props.HostNames {
				if hostname != nil && props.DefaultHostName != nil && *hostname != *props.DefaultHostName {
					customDomains = append(customDomains, *hostname)
				}
			}
			if len(customDomains) > 0 {
				metadata["custom_domains"] = strings.Join(customDomains, ",")
			}
		}

		if props.SiteConfig != nil {
			if props.SiteConfig.LinuxFxVersion != nil {
				schema.AddMetadata(metadata, "linux_fx_version", props.SiteConfig.LinuxFxVersion)
			}
			if props.SiteConfig.WindowsFxVersion != nil {
				schema.AddMetadata(metadata, "windows_fx_version", props.SiteConfig.WindowsFxVersion)
			}
			if props.SiteConfig.AlwaysOn != nil {
				metadata["always_on"] = fmt.Sprintf("%v", *props.SiteConfig.AlwaysOn)
			}
			if props.SiteConfig.FtpsState != nil {
				ftpsState := string(*props.SiteConfig.FtpsState)
				metadata["ftps_state"] = ftpsState
			}
			if props.SiteConfig.MinTLSVersion != nil {
				tlsVersion := string(*props.SiteConfig.MinTLSVersion)
				metadata["min_tls_version"] = tlsVersion
			}
		}

		if props.AvailabilityState != nil {
			availabilityState := string(*props.AvailabilityState)
			metadata["availability_state"] = availabilityState
		}

		if props.UsageState != nil {
			usageState := string(*props.UsageState)
			metadata["usage_state"] = usageState
		}

		if props.IsDefaultContainer != nil {
			metadata["is_default_container"] = fmt.Sprintf("%v", *props.IsDefaultContainer)
		}

		if props.RedundancyMode != nil {
			redundancyMode := string(*props.RedundancyMode)
			metadata["redundancy_mode"] = redundancyMode
		}

		if props.IsXenon != nil {
			metadata["is_xenon"] = fmt.Sprintf("%v", *props.IsXenon)
		}

		if props.HyperV != nil {
			metadata["hyper_v"] = fmt.Sprintf("%v", *props.HyperV)
		}
	}

	if app.Identity != nil && app.Identity.Type != nil {
		identityType := string(*app.Identity.Type)
		schema.AddMetadata(metadata, "identity_type", &identityType)
		if app.Identity.PrincipalID != nil {
			metadata["identity_principal_id"] = *app.Identity.PrincipalID
		}
	}

	if len(app.Tags) > 0 {
		if tagString := buildAzureTagString(app.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}

func (asp *appServiceProvider) getDeploymentSlotMetadata(slot *armappservice.Site, parentApp *armappservice.Site) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "slot_name", slot.Name)
	schema.AddMetadata(metadata, "slot_id", slot.ID)
	metadata["subscription_id"] = asp.SubscriptionID
	metadata["owner_id"] = asp.SubscriptionID
	schema.AddMetadata(metadata, "location", slot.Location)
	schema.AddMetadata(metadata, "kind", slot.Kind)
	metadata["is_deployment_slot"] = "true"

	if parentApp != nil && parentApp.Name != nil {
		schema.AddMetadata(metadata, "parent_app_name", parentApp.Name)
	}

	// Parse resource group from ID
	if slot.ID != nil {
		parts := strings.Split(*slot.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	if slot.Properties != nil {
		props := slot.Properties

		schema.AddMetadata(metadata, "state", props.State)
		schema.AddMetadata(metadata, "default_hostname", props.DefaultHostName)

		if props.Enabled != nil {
			metadata["enabled"] = fmt.Sprintf("%v", *props.Enabled)
		}

		if props.HTTPSOnly != nil {
			metadata["https_only"] = fmt.Sprintf("%v", *props.HTTPSOnly)
		}

		if props.ClientAffinityEnabled != nil {
			metadata["client_affinity_enabled"] = fmt.Sprintf("%v", *props.ClientAffinityEnabled)
		}

		schema.AddMetadata(metadata, "app_service_plan_id", props.ServerFarmID)
	}

	if len(slot.Tags) > 0 {
		if tagString := buildAzureTagString(slot.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}
