package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

type publicIPProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential // Track 2: replaced autorest.Authorizer
	extendedMetadata bool
}

func (pip *publicIPProvider) name() string {
	return "publicip"
}

// GetResource returns all the resources in the store for a provider.
func (pip *publicIPProvider) GetResource(ctx context.Context) (*schema.Resources, error) {

	list := schema.NewResources()

	ips, err := pip.fetchPublicIPs(ctx)
	if err != nil {
		return nil, err
	}

	for _, ip := range ips {
		if ip.Properties == nil || ip.Properties.IPAddress == nil {
			continue
		}

		var metadata map[string]string
		if pip.extendedMetadata {
			metadata = pip.getPublicIPMetadata(ip)
		}

		resource := &schema.Resource{
			Provider: providerName,
			ID:       pip.id,
			Public:   true,
			Service:  pip.name(),
			Metadata: metadata,
		}

		// Track 2: Use pointer dereference safely
		if ip.Properties.PublicIPAddressVersion != nil && *ip.Properties.PublicIPAddressVersion == armnetwork.IPVersionIPv4 {
			resource.PublicIPv4 = *ip.Properties.IPAddress
		} else if ip.Properties.PublicIPAddressVersion != nil {
			resource.PublicIPv6 = *ip.Properties.IPAddress
		}

		list.Append(resource)

		if pip.extendedMetadata && ip.Properties.DNSSettings != nil && ip.Properties.DNSSettings.Fqdn != nil {
			dnsResource := &schema.Resource{
				Provider: providerName,
				ID:       pip.id,
				DNSName:  *ip.Properties.DNSSettings.Fqdn,
				Service:  pip.name(),
			}
			if metadata != nil {
				dnsResource.Metadata = make(map[string]string)
				for k, v := range metadata {
					dnsResource.Metadata[k] = v
				}
			}
			list.Append(dnsResource)
		}
	}
	return list, nil
}

func (pip *publicIPProvider) fetchPublicIPs(ctx context.Context) ([]*armnetwork.PublicIPAddress, error) {
	var ips []*armnetwork.PublicIPAddress

	// Track 2: Create public IP addresses client directly
	ipClient, err := armnetwork.NewPublicIPAddressesClient(pip.SubscriptionID, pip.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create public IP client: %w", err)
	}

	// Track 2: Use pager pattern instead of iterator
	pager := ipClient.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list public IPs: %w", err)
		}

		ips = append(ips, page.Value...)
	}

	return ips, nil
}

func (pip *publicIPProvider) getPublicIPMetadata(ip *armnetwork.PublicIPAddress) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "public_ip_name", ip.Name)
	schema.AddMetadata(metadata, "public_ip_id", ip.ID)
	metadata["subscription_id"] = pip.SubscriptionID
	metadata["owner_id"] = pip.SubscriptionID
	schema.AddMetadata(metadata, "location", ip.Location)

	// Track 2: Parse resource group from ID manually (no ParseResourceID in Track 2)
	if ip.ID != nil {
		// Azure resource ID format: /subscriptions/{subId}/resourceGroups/{rgName}/...
		parts := strings.Split(*ip.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	if ip.Properties != nil {
		props := ip.Properties

		if props.ProvisioningState != nil {
			provisioningState := string(*props.ProvisioningState)
			schema.AddMetadata(metadata, "provisioning_state", &provisioningState)
		}

		if props.PublicIPAllocationMethod != nil {
			allocationMethod := string(*props.PublicIPAllocationMethod)
			metadata["allocation_method"] = allocationMethod
		}

		if props.PublicIPAddressVersion != nil {
			ipVersion := string(*props.PublicIPAddressVersion)
			metadata["ip_version"] = ipVersion
		}

		if props.IdleTimeoutInMinutes != nil {
			metadata["idle_timeout_in_minutes"] = fmt.Sprintf("%d", *props.IdleTimeoutInMinutes)
		}

		if props.DNSSettings != nil {
			schema.AddMetadata(metadata, "dns_domain_name_label", props.DNSSettings.DomainNameLabel)
			schema.AddMetadata(metadata, "dns_fqdn", props.DNSSettings.Fqdn)
			schema.AddMetadata(metadata, "dns_reverse_fqdn", props.DNSSettings.ReverseFqdn)
		}

		if props.IPConfiguration != nil {
			schema.AddMetadata(metadata, "associated_ip_config_id", props.IPConfiguration.ID)
		}

		if props.PublicIPPrefix != nil {
			schema.AddMetadata(metadata, "public_ip_prefix_id", props.PublicIPPrefix.ID)
		}

		if props.DdosSettings != nil {
			if props.DdosSettings.ProtectedIP != nil {
				protected := fmt.Sprintf("%v", *props.DdosSettings.ProtectedIP)
				metadata["ddos_protected_ip"] = protected
			}
		}

		if props.IPTags != nil && len(props.IPTags) > 0 {
			var ipTagStrings []string
			for _, ipTag := range props.IPTags {
				if ipTag.IPTagType != nil && ipTag.Tag != nil {
					ipTagStrings = append(ipTagStrings, fmt.Sprintf("%s:%s", *ipTag.IPTagType, *ipTag.Tag))
				}
			}
			if len(ipTagStrings) > 0 {
				metadata["ip_tags"] = strings.Join(ipTagStrings, ",")
			}
		}

		if props.LinkedPublicIPAddress != nil {
			schema.AddMetadata(metadata, "linked_public_ip_id", props.LinkedPublicIPAddress.ID)
		}

		if props.MigrationPhase != nil {
			phase := string(*props.MigrationPhase)
			metadata["migration_phase"] = phase
		}

		if props.NatGateway != nil {
			schema.AddMetadata(metadata, "nat_gateway_id", props.NatGateway.ID)
		}

		if props.ServicePublicIPAddress != nil {
			schema.AddMetadata(metadata, "service_public_ip_id", props.ServicePublicIPAddress.ID)
		}

		if props.DeleteOption != nil {
			deleteOption := string(*props.DeleteOption)
			metadata["delete_option"] = deleteOption
		}
	}

	if ip.SKU != nil && ip.SKU.Name != nil {
		skuName := string(*ip.SKU.Name)
		schema.AddMetadata(metadata, "sku_name", &skuName)

		if ip.SKU.Tier != nil {
			tier := string(*ip.SKU.Tier)
			metadata["sku_tier"] = tier
		}
	}

	if len(ip.Zones) > 0 {
		var zones []string
		for _, z := range ip.Zones {
			if z != nil {
				zones = append(zones, *z)
			}
		}
		if len(zones) > 0 {
			metadata["availability_zones"] = strings.Join(zones, ",")
		}
	}

	if len(ip.Tags) > 0 {
		if tagString := buildAzureTagString(ip.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	if ip.ExtendedLocation != nil {
		schema.AddMetadata(metadata, "extended_location_name", ip.ExtendedLocation.Name)
		if ip.ExtendedLocation.Type != nil {
			extType := string(*ip.ExtendedLocation.Type)
			schema.AddMetadata(metadata, "extended_location_type", &extType)
		}
	}

	return metadata
}
