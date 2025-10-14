package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// functionsProvider is a provider for Azure Functions
type functionsProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (fp *functionsProvider) name() string {
	return "functions"
}

// GetResource returns all the Azure Functions resources.
func (fp *functionsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	apps, err := fp.fetchFunctionApps(ctx)
	if err != nil {
		return nil, err
	}

	for _, app := range apps {
		// Skip if not a function app
		if app.Kind == nil || !strings.Contains(strings.ToLower(*app.Kind), "functionapp") {
			continue
		}

		// Default hostname: <function-name>.azurewebsites.net
		var defaultHostname string
		if app.Name != nil {
			defaultHostname = fmt.Sprintf("%s.azurewebsites.net", *app.Name)
		}

		var metadata map[string]string
		if fp.extendedMetadata {
			metadata = fp.getFunctionAppMetadata(app, defaultHostname)
		}

		// Add resource for default hostname
		if defaultHostname != "" {
			resource := &schema.Resource{
				Provider: providerName,
				ID:       fp.id,
				DNSName:  defaultHostname,
				Service:  fp.name(),
				Metadata: metadata,
			}
			list.Append(resource)
		}

		// Add resources for custom domains if configured
		if app.Properties != nil && app.Properties.HostNames != nil {
			for _, hostname := range app.Properties.HostNames {
				if hostname != nil && *hostname != defaultHostname {
					customResource := &schema.Resource{
						Provider: providerName,
						ID:       fp.id,
						DNSName:  *hostname,
						Service:  fp.name(),
					}
					if metadata != nil {
						customResource.Metadata = make(map[string]string)
						for k, v := range metadata {
							customResource.Metadata[k] = v
						}
					}
					list.Append(customResource)
				}
			}
		}
	}
	return list, nil
}

func (fp *functionsProvider) fetchFunctionApps(ctx context.Context) ([]*armappservice.Site, error) {
	// Create web apps client
	client, err := armappservice.NewWebAppsClient(fp.SubscriptionID, fp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create web apps client: %w", err)
	}

	// Use pager pattern to list all apps
	var apps []*armappservice.Site
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list web apps: %w", err)
		}

		apps = append(apps, page.Value...)
	}

	return apps, nil
}

func (fp *functionsProvider) getFunctionAppMetadata(app *armappservice.Site, defaultHostname string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "function_app_name", app.Name)
	schema.AddMetadata(metadata, "function_app_id", app.ID)
	metadata["subscription_id"] = fp.SubscriptionID
	metadata["owner_id"] = fp.SubscriptionID
	schema.AddMetadata(metadata, "location", app.Location)
	schema.AddMetadata(metadata, "kind", app.Kind)
	schema.AddMetadata(metadata, "type", app.Type)

	// Parse resource group from ID
	if app.ID != nil {
		_, resourceGroup := parseAzureResourceID(*app.ID)
		if resourceGroup != "" {
			metadata["resource_group"] = resourceGroup
		}
	}

	if app.Properties != nil {
		props := app.Properties

		if props.State != nil {
			metadata["state"] = *props.State
		}

		if props.DefaultHostName != nil {
			metadata["default_hostname"] = *props.DefaultHostName
		} else if defaultHostname != "" {
			metadata["default_hostname"] = defaultHostname
		}

		if props.Enabled != nil {
			metadata["enabled"] = fmt.Sprintf("%v", *props.Enabled)
		}

		if props.HTTPSOnly != nil {
			metadata["https_only"] = fmt.Sprintf("%v", *props.HTTPSOnly)
		}

		if len(props.HostNames) > 0 {
			var hostnames []string
			for _, hostname := range props.HostNames {
				if hostname != nil {
					hostnames = append(hostnames, *hostname)
				}
			}
			if len(hostnames) > 0 {
				metadata["custom_domains"] = strings.Join(hostnames, ",")
			}
		}

		schema.AddMetadata(metadata, "repository_site_name", props.RepositorySiteName)

		if props.UsageState != nil {
			usageState := string(*props.UsageState)
			metadata["usage_state"] = usageState
		}

		if props.AvailabilityState != nil {
			availabilityState := string(*props.AvailabilityState)
			metadata["availability_state"] = availabilityState
		}

		if props.ServerFarmID != nil {
			schema.AddMetadata(metadata, "app_service_plan_id", props.ServerFarmID)
		}

		if props.SiteConfig != nil {
			config := props.SiteConfig

			if config.LinuxFxVersion != nil {
				metadata["runtime_stack"] = *config.LinuxFxVersion
			}
			if config.WindowsFxVersion != nil && metadata["runtime_stack"] == "" {
				metadata["runtime_stack"] = *config.WindowsFxVersion
			}

			if config.NetFrameworkVersion != nil {
				metadata["runtime_version"] = *config.NetFrameworkVersion
			}
			if config.NodeVersion != nil && metadata["runtime_version"] == "" {
				metadata["runtime_version"] = *config.NodeVersion
			}
			if config.PythonVersion != nil && metadata["runtime_version"] == "" {
				metadata["runtime_version"] = *config.PythonVersion
			}
			if config.JavaVersion != nil && metadata["runtime_version"] == "" {
				metadata["runtime_version"] = *config.JavaVersion
			}
			if config.PhpVersion != nil && metadata["runtime_version"] == "" {
				metadata["runtime_version"] = *config.PhpVersion
			}

			if config.AlwaysOn != nil {
				metadata["always_on"] = fmt.Sprintf("%v", *config.AlwaysOn)
			}

			if config.FtpsState != nil {
				ftpsState := string(*config.FtpsState)
				metadata["ftps_state"] = ftpsState
			}

			if config.MinTLSVersion != nil {
				minTLS := string(*config.MinTLSVersion)
				metadata["min_tls_version"] = minTLS
			}
		}

		if props.DailyMemoryTimeQuota != nil {
			metadata["daily_memory_time_quota"] = fmt.Sprintf("%d", *props.DailyMemoryTimeQuota)
		}

		schema.AddMetadata(metadata, "outbound_ip_addresses", props.OutboundIPAddresses)
		schema.AddMetadata(metadata, "possible_outbound_ip_addresses", props.PossibleOutboundIPAddresses)

		if props.ClientCertEnabled != nil {
			metadata["client_cert_enabled"] = fmt.Sprintf("%v", *props.ClientCertEnabled)
		}

		if props.ClientCertMode != nil {
			clientCertMode := string(*props.ClientCertMode)
			metadata["client_cert_mode"] = clientCertMode
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
		metadata["identity_type"] = identityType

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
