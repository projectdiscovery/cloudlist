package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/network/mgmt/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

type publicIPProvider struct {
	id               string
	SubscriptionID   string
	Authorizer       autorest.Authorizer
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
		if ip.IPAddress == nil {
			continue
		}

		var metadata map[string]string
		if pip.extendedMetadata {
			metadata = pip.getPublicIPMetadata(&ip)
		}

		resource := &schema.Resource{
			Provider: providerName,
			ID:       pip.id,
			Public:   true,
			Service:  pip.name(),
			Metadata: metadata,
		}

		if ip.PublicIPAddressVersion == network.IPv4 {
			resource.PublicIPv4 = *ip.IPAddress
		} else {
			resource.PublicIPv6 = *ip.IPAddress
		}

		list.Append(resource)

		if pip.extendedMetadata && ip.DNSSettings != nil && ip.DNSSettings.Fqdn != nil {
			dnsResource := &schema.Resource{
				Provider: providerName,
				ID:       pip.id,
				DNSName:  *ip.DNSSettings.Fqdn,
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

func (pip *publicIPProvider) fetchPublicIPs(ctx context.Context) ([]network.PublicIPAddress, error) {
	var ips []network.PublicIPAddress

	ipClient := network.NewPublicIPAddressesClient(pip.SubscriptionID)
	ipClient.Authorizer = pip.Authorizer

	ipsIt, err := ipClient.ListAllComplete(ctx)
	if err != nil {
		return nil, err
	}

	for ipsIt.NotDone() {
		ip := ipsIt.Value()
		ips = append(ips, ip)

		if err = ipsIt.NextWithContext(ctx); err != nil {
			return ips, err
		}
	}

	return ips, err
}

func (pip *publicIPProvider) getPublicIPMetadata(ip *network.PublicIPAddress) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "public_ip_name", ip.Name)
	schema.AddMetadata(metadata, "public_ip_id", ip.ID)
	metadata["subscription_id"] = pip.SubscriptionID
	metadata["owner_id"] = pip.SubscriptionID
	schema.AddMetadata(metadata, "location", ip.Location)

	if ip.ID != nil {
		res, err := azure.ParseResourceID(*ip.ID)
		if err == nil {
			metadata["resource_group"] = res.ResourceGroup
		}
	}

	if ip.PublicIPAddressPropertiesFormat != nil {
		props := ip.PublicIPAddressPropertiesFormat

		if props.ProvisioningState != "" {
			provisioningState := string(props.ProvisioningState)
			schema.AddMetadata(metadata, "provisioning_state", &provisioningState)
		}

		if props.PublicIPAllocationMethod != "" {
			allocationMethod := string(props.PublicIPAllocationMethod)
			metadata["allocation_method"] = allocationMethod
		}

		if props.PublicIPAddressVersion != "" {
			ipVersion := string(props.PublicIPAddressVersion)
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
			if props.DdosSettings.ProtectionMode != "" {
				mode := string(props.DdosSettings.ProtectionMode)
				metadata["ddos_protection_mode"] = mode
			}
			if props.DdosSettings.DdosProtectionPlan != nil {
				schema.AddMetadata(metadata, "ddos_protection_plan_id", props.DdosSettings.DdosProtectionPlan.ID)
			}
		}

		if props.IPTags != nil && len(*props.IPTags) > 0 {
			var ipTagStrings []string
			for _, ipTag := range *props.IPTags {
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

		if props.MigrationPhase != "" {
			phase := string(props.MigrationPhase)
			metadata["migration_phase"] = phase
		}

		if props.NatGateway != nil {
			schema.AddMetadata(metadata, "nat_gateway_id", props.NatGateway.ID)
		}

		if props.ServicePublicIPAddress != nil {
			schema.AddMetadata(metadata, "service_public_ip_id", props.ServicePublicIPAddress.ID)
		}

		if props.DeleteOption != "" {
			deleteOption := string(props.DeleteOption)
			metadata["delete_option"] = deleteOption
		}
	}

	if ip.Sku != nil {
		skuName := string(ip.Sku.Name)
		schema.AddMetadata(metadata, "sku_name", &skuName)

		if ip.Sku.Tier != "" {
			tier := string(ip.Sku.Tier)
			metadata["sku_tier"] = tier
		}
	}

	if ip.Zones != nil && len(*ip.Zones) > 0 {
		zones := strings.Join(*ip.Zones, ",")
		metadata["availability_zones"] = zones
	}

	if len(ip.Tags) > 0 {
		if tagString := buildAzureTagString(ip.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	if ip.ExtendedLocation != nil {
		schema.AddMetadata(metadata, "extended_location_name", ip.ExtendedLocation.Name)
		if ip.ExtendedLocation.Type != "" {
			extType := string(ip.ExtendedLocation.Type)
			schema.AddMetadata(metadata, "extended_location_type", &extType)
		}
	}

	return metadata
}
