package linode

import (
	"context"

	"github.com/linode/linodego"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for linode API
type instanceProvider struct {
	profile string
	client  *linodego.Client
}

// GetResource returns all the instance resources for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	opts := &linodego.PageOptions{
		Page: 200,
	}
	opt := &linodego.ListOptions{PageOptions: opts}

	instances, err := d.client.ListInstances(ctx, opt)
	if err != nil {
		return nil, err
	}

	list := &schema.Resources{}

	for _, inst := range instances {

		ip4 := inst.IPv4[0].String()

		//TODO@sajad: check if whole IPv4 list is to be usd

		// var ipStrList []string
		// for _, ip := range inst.IPv4 {
		// 	ipStrList = append(ipStrList, string(*ip))
		// }
		// strings.Join(ipStrList, ",")

		list.Append(&schema.Resource{
			Provider:   providerName,
			PublicIPv4: ip4,
			Profile:    d.profile,
			Public:     ip4 != "",
		})
	}

	return list, nil
}
