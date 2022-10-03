package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/network/mgmt/network"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// vmProvider is an instance provider for Azure API
type vmProvider struct {
	id             string
	SubscriptionID string
	Authorizer     autorest.Authorizer
}

// GetResource returns all the resources in the store for a provider.
func (d *vmProvider) GetResource(ctx context.Context) (*schema.Resources, error) {

	list := schema.NewResources()

	groups, err := fetchResouceGroups(ctx, d)
	if err != nil {
		return nil, err
	}

	for _, group := range groups {
		vmList, err := fetchVMList(ctx, group, d)
		if err != nil {
			return nil, err
		}

		for _, vm := range vmList {
			nics := *vm.NetworkProfile.NetworkInterfaces
			for _, nic := range nics {

				res, err := azure.ParseResourceID(*nic.ID)
				if err != nil {
					return nil, err
				}

				ipconfigList, err := fetchIPConfigList(ctx, group, res.ResourceName, d)
				if err != nil {
					return nil, err
				}
				for _, ipConfig := range ipconfigList {
					privateIP := *ipConfig.PrivateIPAddress

					res, err := azure.ParseResourceID(*ipConfig.PublicIPAddress.ID)
					if err != nil {
						return nil, err
					}

					publicIP, err := fetchPublicIP(ctx, group, res.ResourceName, d)
					if err != nil {
						return nil, err
					}

					list.Append(&schema.Resource{
						Provider:    providerName,
						PublicIPv4:  *publicIP.IPAddress,
						ID:          d.id,
						PrivateIpv4: privateIP,
					})
				}
			}
		}
	}
	return list, nil
}

func fetchResouceGroups(ctx context.Context, sess *vmProvider) (resGrpList []string, err error) {

	grClient := resources.NewGroupsClient(sess.SubscriptionID)
	grClient.Authorizer = sess.Authorizer

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
