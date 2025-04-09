package azure

import (
	"context"
	"sync"

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
	id             string
	SubscriptionID string
	Authorizer     autorest.Authorizer
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
					continue
				}

				res, err := azure.ParseResourceID(*ipConfig.PublicIPAddress.ID)
				if err != nil {
					gologger.Warning().Msgf("error parsing resource ID: %s", err)
					continue
				}

				publicIP, err := fetchPublicIP(ctx, group, res.ResourceName, d)
				if err != nil {
					gologger.Warning().Msgf("error fetching public IP %s: %s", res.ResourceName, err)
					continue
				}

				if publicIP.IPAddress == nil {
					continue
				}

				resource := &schema.Resource{
					Provider:    providerName,
					ID:          d.id,
					PrivateIpv4: *ipConfig.PrivateIPAddress,
					Service:     d.name(),
				}

				if publicIP.PublicIPAddressVersion == network.IPv4 {
					resource.PublicIPv4 = *publicIP.IPAddress
				} else {
					resource.PublicIPv6 = *publicIP.IPAddress
				}

				resources = append(resources, resource)

				if publicIP.DNSSettings.Fqdn != nil {
					resources = append(resources, &schema.Resource{
						Provider: providerName,
						ID:       d.id,
						DNSName:  *publicIP.DNSSettings.Fqdn,
						Service:  d.name(),
					})
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
