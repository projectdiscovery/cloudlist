package azure

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/alitto/pond/v2"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// vmProvider is an instance provider for Azure API using Track 2 SDK
type vmProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential // Track 2: replaced autorest.Authorizer
	extendedMetadata bool
}

func (d *vmProvider) name() string {
	return "vm"
}

// GetResource returns all the resources in the store for a provider.
func (d *vmProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	mu := &sync.Mutex{}

	groups, err := fetchResourceGroups(ctx, d.SubscriptionID, d.Credential)
	if err != nil {
		return nil, err
	}

	// Create a goroutine pool with size that matches Azure API limitations
	// Adjust pool size based on your Azure throttling limits
	pool := pond.NewPool(10)

	for _, group := range groups {
		group := group // Create local copy for goroutine

		// Submit task to the pool
		pool.Submit(func() {
			resourcesSlice, err := d.processResourceGroup(ctx, group)
			if err != nil {
				gologger.Warning().Msgf("error processing resource group %s: %s", group, err)
			}

			mu.Lock()
			for _, resource := range resourcesSlice {
				list.Append(resource)
			}
			mu.Unlock()
		})
	}
	pool.StopAndWait()

	return list, nil
}

func (d *vmProvider) processResourceGroup(ctx context.Context, group string) ([]*schema.Resource, error) {
	vmList, err := fetchVMList(ctx, group, d)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching vm list")
	}

	var resources []*schema.Resource
	for _, vm := range vmList {
		if vm.Properties == nil || vm.Properties.NetworkProfile == nil || vm.Properties.NetworkProfile.NetworkInterfaces == nil {
			continue
		}

		nics := vm.Properties.NetworkProfile.NetworkInterfaces

		for _, nic := range nics {
			if nic.ID == nil {
				continue
			}

			// Track 2: Parse resource ID manually
			nicName, nicRG := parseAzureResourceID(*nic.ID)
			if nicName == "" || nicRG == "" {
				gologger.Warning().Msgf("error parsing NIC resource ID: %s", *nic.ID)
				continue
			}

			ipconfigList, err := fetchIPConfigList(ctx, nicRG, nicName, d)
			if err != nil {
				gologger.Warning().Msgf("error fetching IP configs for NIC %s: %s", nicName, err)
				continue
			}

			for _, ipConfig := range ipconfigList {
				if ipConfig.Properties == nil || ipConfig.Properties.PublicIPAddress == nil || ipConfig.Properties.PublicIPAddress.ID == nil {
					continue
				}

				pipName, pipRG := parseAzureResourceID(*ipConfig.Properties.PublicIPAddress.ID)
				if pipName == "" || pipRG == "" {
					gologger.Warning().Msgf("error parsing public IP resource ID: %s", *ipConfig.Properties.PublicIPAddress.ID)
					continue
				}

				publicIP, err := fetchPublicIP(ctx, pipRG, pipName, d)
				if err != nil {
					gologger.Warning().Msgf("error fetching public IP %s: %s", pipName, err)
					continue
				}

				if publicIP.Properties == nil || publicIP.Properties.IPAddress == nil {
					continue
				}

				resource := &schema.Resource{
					Provider: providerName,
					ID:       d.id,
					Service:  d.name(),
				}

				// Add private IP if available
				if ipConfig.Properties.PrivateIPAddress != nil {
					// Check IP version for private IP similar to public IP
					privateIPStr := *ipConfig.Properties.PrivateIPAddress
					if ipConfig.Properties.PrivateIPAddressVersion != nil && *ipConfig.Properties.PrivateIPAddressVersion == armnetwork.IPVersionIPv6 {
						resource.PrivateIpv6 = privateIPStr
					} else {
						// Default to IPv4 if not specified
						resource.PrivateIpv4 = privateIPStr
					}
				}

				var metadata map[string]string
				if d.extendedMetadata {
					metadata = d.getVMMetadata(vm, group)
				}
				resource.Metadata = metadata

				// Track 2: Check IP version
				if publicIP.Properties.PublicIPAddressVersion != nil && *publicIP.Properties.PublicIPAddressVersion == armnetwork.IPVersionIPv4 {
					resource.PublicIPv4 = *publicIP.Properties.IPAddress
				} else if publicIP.Properties.PublicIPAddressVersion != nil {
					resource.PublicIPv6 = *publicIP.Properties.IPAddress
				} else {
					// Default to IPv4 if not specified
					resource.PublicIPv4 = *publicIP.Properties.IPAddress
				}

				resources = append(resources, resource)

				// Add DNS resource if available
				if publicIP.Properties.DNSSettings != nil && publicIP.Properties.DNSSettings.Fqdn != nil {
					dnsResource := &schema.Resource{
						Provider: providerName,
						ID:       d.id,
						DNSName:  *publicIP.Properties.DNSSettings.Fqdn,
						Service:  d.name(),
					}
					if metadata != nil {
						dnsResource.Metadata = make(map[string]string)
						for k, v := range metadata {
							dnsResource.Metadata[k] = v
						}
					}
					resources = append(resources, dnsResource)
				}
			}
		}
	}
	return resources, nil
}

// parseAzureResourceID parses an Azure resource ID and returns the resource name and resource group
// Azure resource ID format: /subscriptions/{subId}/resourceGroups/{rgName}/providers/{provider}/{type}/{name}
func parseAzureResourceID(resourceID string) (resourceName, resourceGroup string) {
	parts := strings.Split(resourceID, "/")
	for i, part := range parts {
		if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
			resourceGroup = parts[i+1]
		}
	}
	// The resource name is the last part
	if len(parts) > 0 {
		resourceName = parts[len(parts)-1]
	}
	return resourceName, resourceGroup
}

func fetchResourceGroups(ctx context.Context, subscriptionID string, credential azcore.TokenCredential) (resGrpList []string, err error) {
	// Track 2: Create resource groups client directly
	grClient, err := armresources.NewResourceGroupsClient(subscriptionID, credential, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create resource groups client")
	}

	// Track 2: Use pager pattern
	pager := grClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "error listing resource groups")
		}

		for _, rg := range page.Value {
			if rg.Name != nil {
				resGrpList = append(resGrpList, *rg.Name)
			}
		}
	}
	return resGrpList, nil
}

func fetchVMList(ctx context.Context, group string, sess *vmProvider) (VMList []*armcompute.VirtualMachine, err error) {
	// Track 2: Create virtual machines client directly
	vmClient, err := armcompute.NewVirtualMachinesClient(sess.SubscriptionID, sess.Credential, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create virtual machines client")
	}

	// Track 2: Use pager pattern
	pager := vmClient.NewListPager(group, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "error listing VMs")
		}

		VMList = append(VMList, page.Value...)
	}
	return VMList, nil
}

func fetchIPConfigList(ctx context.Context, group, nic string, sess *vmProvider) (IPConfigList []*armnetwork.InterfaceIPConfiguration, err error) {
	// Track 2: Create network interfaces client directly
	nicClient, err := armnetwork.NewInterfacesClient(sess.SubscriptionID, sess.Credential, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create network interfaces client")
	}

	nicResp, err := nicClient.Get(ctx, group, nic, nil)
	if err != nil {
		return nil, err
	}

	// Track 2: Unwrap Interface from response wrapper
	if nicResp.Interface != nil && nicResp.Interface.Properties != nil && nicResp.Interface.Properties.IPConfigurations != nil {
		IPConfigList = nicResp.Interface.Properties.IPConfigurations
	}

	return IPConfigList, nil
}

func fetchPublicIP(ctx context.Context, group, publicIP string, sess *vmProvider) (IP armnetwork.PublicIPAddress, err error) {
	// Track 2: Create public IP addresses client directly
	ipClient, err := armnetwork.NewPublicIPAddressesClient(sess.SubscriptionID, sess.Credential, nil)
	if err != nil {
		return armnetwork.PublicIPAddress{}, errors.Wrap(err, "failed to create public IP client")
	}

	resp, err := ipClient.Get(ctx, group, publicIP, nil)
	if err != nil {
		return armnetwork.PublicIPAddress{}, err
	}

	return resp.PublicIPAddress, nil
}

func (d *vmProvider) getVMMetadata(vm *armcompute.VirtualMachine, resourceGroup string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "vm_name", vm.Name)
	schema.AddMetadata(metadata, "vm_id", vm.ID)
	metadata["resource_group"] = resourceGroup
	metadata["subscription_id"] = d.SubscriptionID
	schema.AddMetadata(metadata, "location", vm.Location)

	metadata["owner_id"] = d.SubscriptionID

	if vm.Properties != nil {
		// Hardware profile
		if vm.Properties.HardwareProfile != nil && vm.Properties.HardwareProfile.VMSize != nil {
			vmSize := string(*vm.Properties.HardwareProfile.VMSize)
			schema.AddMetadata(metadata, "vm_size", &vmSize)
		}

		schema.AddMetadata(metadata, "provisioning_state", vm.Properties.ProvisioningState)
		schema.AddMetadata(metadata, "vm_id_internal", vm.Properties.VMID)
		schema.AddMetadata(metadata, "license_type", vm.Properties.LicenseType)

		if vm.Properties.TimeCreated != nil {
			metadata["creation_time"] = vm.Properties.TimeCreated.Format(time.RFC3339)
		}

		if vm.Properties.OSProfile != nil {
			schema.AddMetadata(metadata, "computer_name", vm.Properties.OSProfile.ComputerName)
			schema.AddMetadata(metadata, "admin_username", vm.Properties.OSProfile.AdminUsername)
		}

		if vm.Properties.StorageProfile != nil {
			if vm.Properties.StorageProfile.OSDisk != nil {
				if vm.Properties.StorageProfile.OSDisk.OSType != nil {
					osType := string(*vm.Properties.StorageProfile.OSDisk.OSType)
					schema.AddMetadata(metadata, "os_type", &osType)
				}
				schema.AddMetadata(metadata, "os_disk_name", vm.Properties.StorageProfile.OSDisk.Name)
			}
			if vm.Properties.StorageProfile.ImageReference != nil {
				schema.AddMetadata(metadata, "image_publisher", vm.Properties.StorageProfile.ImageReference.Publisher)
				schema.AddMetadata(metadata, "image_offer", vm.Properties.StorageProfile.ImageReference.Offer)
				schema.AddMetadata(metadata, "image_sku", vm.Properties.StorageProfile.ImageReference.SKU)
				schema.AddMetadata(metadata, "image_version", vm.Properties.StorageProfile.ImageReference.Version)
			}
		}

		if vm.Properties.AvailabilitySet != nil {
			schema.AddMetadata(metadata, "availability_set_id", vm.Properties.AvailabilitySet.ID)
		}

		if vm.Properties.VirtualMachineScaleSet != nil {
			schema.AddMetadata(metadata, "vmss_id", vm.Properties.VirtualMachineScaleSet.ID)
		}
	}

	if len(vm.Zones) > 0 {
		var zones []string
		for _, z := range vm.Zones {
			if z != nil {
				zones = append(zones, *z)
			}
		}
		if len(zones) > 0 {
			metadata["availability_zones"] = strings.Join(zones, ",")
		}
	}

	if len(vm.Tags) > 0 {
		if tagString := buildAzureTagString(vm.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	if vm.Identity != nil && vm.Identity.Type != nil {
		identityType := string(*vm.Identity.Type)
		schema.AddMetadata(metadata, "identity_type", &identityType)
		if vm.Identity.PrincipalID != nil {
			metadata["identity_principal_id"] = *vm.Identity.PrincipalID
		}
	}

	if vm.Plan != nil {
		schema.AddMetadata(metadata, "plan_name", vm.Plan.Name)
		schema.AddMetadata(metadata, "plan_publisher", vm.Plan.Publisher)
		schema.AddMetadata(metadata, "plan_product", vm.Plan.Product)
	}

	return metadata
}

func buildAzureTagString(tags map[string]*string) string {
	var tagPairs []string
	for key, value := range tags {
		if value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", key, *value))
		}
	}
	return strings.Join(tagPairs, ",")
}
