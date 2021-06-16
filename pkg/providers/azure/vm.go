package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// vmProvider is an instance provider for Azure API
type vmProvider struct {
	profile        string
	SubscriptionID string
	Authorizer     autorest.Authorizer
}

// GetResource returns all the resources in the store for a provider.
func (d *vmProvider) GetResource(ctx context.Context) (*schema.Resources, error) {

	list := &schema.Resources{}

	groups, err := getResouceGroups(d)
	if err != nil {
		return nil, err
	}

	vmClient := compute.NewVirtualMachinesClient(d.SubscriptionID)
	vmClient.Authorizer = d.Authorizer

	for _, group := range groups {
		for vm, err := vmClient.ListComplete(context.Background(), group); vm.NotDone(); err = vm.Next() {
			if err != nil {
				return nil, errors.Wrap(err, "error traverising vm list")
			}
			//vm.Value()

			//TODO@sajad: use network package to get ip address of the vm

			list.Append(&schema.Resource{
				Provider: providerName,
				// PublicIPv4:  ,
				Profile: d.profile,
				// PrivateIpv4: ,
				// Public:     ,
			})

		}
	}
	return list, nil
}

func getResouceGroups(sess *vmProvider) (resGrpList []string, err error) {

	grClient := resources.NewGroupsClient(sess.SubscriptionID)
	grClient.Authorizer = sess.Authorizer

	for list, err := grClient.ListComplete(context.Background(), "", nil); list.NotDone(); err = list.Next() {
		if err != nil {
			return nil, errors.Wrap(err, "error traversing resource group list")
		}
		resGrp := *list.Value().Name
		resGrpList = append(resGrpList, resGrp)
	}
	return resGrpList, err
}
