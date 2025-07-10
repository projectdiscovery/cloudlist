package azure

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/network/mgmt/network"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/alitto/pond/v2"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// vmProvider is an instance provider for Azure API
type vmProvider struct {
	id               string
	SubscriptionID   string
	Authorizer       autorest.Authorizer
	extendedMetadata bool
}

func (d *vmProvider) name() string {
	return "vm"
}

// GetResource returns all the resources in the store for a provider.
func (d *vmProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	mu := &sync.Mutex{}

	groups, err := fetchResouceGroups(ctx, d.SubscriptionID, d.Authorizer)
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
		nics := *vm.NetworkProfile.NetworkInterfaces

		for _, nic := range nics {
			res, err := azure.ParseResourceID(*nic.ID)
			if err != nil {
				gologger.Warning().Msgf("error parsing resource ID: %s", err)
				continue
			}

			ipconfigList, err := fetchIPConfigList(ctx, group, res.ResourceName, d)
			if err != nil {
				gologger.Warning().Msgf("error fetching IP configs for NIC %s: %s", res.ResourceName, err)
				continue
			}

			for _, ipConfig := range ipconfigList {
				if ipConfig.PublicIPAddress == nil {
					gologger.Warning().Msgf("no public IP address found for NIC %s", res.ResourceName)
					continue
				}

				res, err := azure.ParseResourceID(*ipConfig.PublicIPAddress.ID)
				if err != nil {
					gologger.Warning().Msgf("error parsing resource ID: %s", err)
					continue
				}

				publicIP, err := fetchPublicIP(ctx, res.ResourceGroup, res.ResourceName, d)
				if err != nil {
					gologger.Warning().Msgf("error fetching public IP %s: %s", res.ResourceName, err)
					continue
				}

				if publicIP.IPAddress == nil {
					gologger.Warning().Msgf("no public IP address found for NIC %s", res.ResourceName)
					continue
				}

				resource := &schema.Resource{
					Provider:    providerName,
					ID:          d.id,
					PrivateIpv4: *ipConfig.PrivateIPAddress,
					Service:     d.name(),
				}

				var metadata map[string]string
				if d.extendedMetadata {
					metadata = d.getVMMetadata(vm, group)
				}
				resource.Metadata = metadata

				if publicIP.PublicIPAddressVersion == network.IPv4 {
					resource.PublicIPv4 = *publicIP.IPAddress
				} else {
					resource.PublicIPv6 = *publicIP.IPAddress
				}

				resources = append(resources, resource)

				if publicIP.DNSSettings != nil && publicIP.DNSSettings.Fqdn != nil {
					dnsResource := &schema.Resource{
						Provider: providerName,
						ID:       d.id,
						DNSName:  *publicIP.DNSSettings.Fqdn,
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

func fetchResouceGroups(ctx context.Context, subscriptionID string, authorizer autorest.Authorizer) (resGrpList []string, err error) {
	grClient := resources.NewGroupsClient(subscriptionID)
	grClient.Authorizer = authorizer

	for list, err := grClient.ListComplete(ctx, "", nil); list.NotDone(); err = list.Next() {

		if err != nil {
			return nil, errors.Wrap(err, "error traversing resource group list")
		}
		resGrp := *list.Value().Name
		resGrpList = append(resGrpList, resGrp)
	}
	return resGrpList, err
}

func fetchVMList(ctx context.Context, group string, sess *vmProvider) (VMList []compute.VirtualMachine, err error) {
	vmClient := compute.NewVirtualMachinesClient(sess.SubscriptionID)
	vmClient.Authorizer = sess.Authorizer

	for vm, err := vmClient.ListComplete(context.Background(), group, ""); vm.NotDone(); err = vm.Next() {
		if err != nil {
			return nil, errors.Wrap(err, "error traverising vm list")
		}
		VMList = append(VMList, vm.Value())
	}
	return VMList, err
}

func fetchIPConfigList(ctx context.Context, group, nic string, sess *vmProvider) (IPConfigList []network.InterfaceIPConfigurationPropertiesFormat, err error) {

	nicClient := network.NewInterfacesClient(sess.SubscriptionID)
	nicClient.Authorizer = sess.Authorizer

	nicRes, err := nicClient.Get(ctx, group, nic, "")
	if err != nil {
		return nil, err
	}

	ipconfigs := *nicRes.IPConfigurations
	for _, v := range ipconfigs {
		IPConfigList = append(IPConfigList, *v.InterfaceIPConfigurationPropertiesFormat)
	}

	return IPConfigList, err
}

func fetchPublicIP(ctx context.Context, group, publicIP string, sess *vmProvider) (IP network.PublicIPAddress, err error) {

	ipClient := network.NewPublicIPAddressesClient(sess.SubscriptionID)
	ipClient.Authorizer = sess.Authorizer

	IP, err = ipClient.Get(ctx, group, publicIP, "")
	if err != nil {
		return network.PublicIPAddress{}, err
	}

	return IP, err
}

func (d *vmProvider) getVMMetadata(vm compute.VirtualMachine, resourceGroup string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "vm_name", vm.Name)
	schema.AddMetadata(metadata, "vm_id", vm.ID)
	metadata["resource_group"] = resourceGroup
	metadata["subscription_id"] = d.SubscriptionID
	schema.AddMetadata(metadata, "location", vm.Location)

	if vm.VirtualMachineProperties != nil && vm.VirtualMachineProperties.HardwareProfile != nil {
		vmSize := string(vm.VirtualMachineProperties.HardwareProfile.VMSize)
		schema.AddMetadata(metadata, "vm_size", &vmSize)
	}

	metadata["owner_id"] = d.SubscriptionID

	if vm.VirtualMachineProperties != nil {
		schema.AddMetadata(metadata, "provisioning_state", vm.VirtualMachineProperties.ProvisioningState)
		schema.AddMetadata(metadata, "vm_id_internal", vm.VirtualMachineProperties.VMID)
		schema.AddMetadata(metadata, "license_type", vm.VirtualMachineProperties.LicenseType)

		if vm.VirtualMachineProperties.TimeCreated != nil {
			metadata["creation_time"] = vm.VirtualMachineProperties.TimeCreated.Format(time.RFC3339)
		}

		if vm.VirtualMachineProperties.OsProfile != nil {
			schema.AddMetadata(metadata, "computer_name", vm.VirtualMachineProperties.OsProfile.ComputerName)
			schema.AddMetadata(metadata, "admin_username", vm.VirtualMachineProperties.OsProfile.AdminUsername)
		}

		if vm.VirtualMachineProperties.StorageProfile != nil {
			if vm.VirtualMachineProperties.StorageProfile.OsDisk != nil {
				osType := string(vm.VirtualMachineProperties.StorageProfile.OsDisk.OsType)
				schema.AddMetadata(metadata, "os_type", &osType)
				schema.AddMetadata(metadata, "os_disk_name", vm.VirtualMachineProperties.StorageProfile.OsDisk.Name)
			}
			if vm.VirtualMachineProperties.StorageProfile.ImageReference != nil {
				schema.AddMetadata(metadata, "image_publisher", vm.VirtualMachineProperties.StorageProfile.ImageReference.Publisher)
				schema.AddMetadata(metadata, "image_offer", vm.VirtualMachineProperties.StorageProfile.ImageReference.Offer)
				schema.AddMetadata(metadata, "image_sku", vm.VirtualMachineProperties.StorageProfile.ImageReference.Sku)
				schema.AddMetadata(metadata, "image_version", vm.VirtualMachineProperties.StorageProfile.ImageReference.Version)
			}
		}

		if vm.VirtualMachineProperties.AvailabilitySet != nil {
			schema.AddMetadata(metadata, "availability_set_id", vm.VirtualMachineProperties.AvailabilitySet.ID)
		}

		if vm.VirtualMachineProperties.VirtualMachineScaleSet != nil {
			schema.AddMetadata(metadata, "vmss_id", vm.VirtualMachineProperties.VirtualMachineScaleSet.ID)
		}
	}

	if vm.Zones != nil && len(*vm.Zones) > 0 {
		zones := strings.Join(*vm.Zones, ",")
		metadata["availability_zones"] = zones
	}

	if len(vm.Tags) > 0 {
		if tagString := buildAzureTagString(vm.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	if vm.Identity != nil {
		identityType := string(vm.Identity.Type)
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
