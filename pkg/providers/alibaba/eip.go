package alibaba

import (
	"context"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// eipProvider is an Elastic IP provider for alibaba API. Unlike the instance
// service, it also finds EIPs that are not bound to an ECS instance.
type eipProvider struct {
	id     string
	client *vpc.Client
}

func (d *eipProvider) name() string {
	return "eip"
}

// GetResource returns all the resources in the store for a provider.
func (d *eipProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	request := vpc.CreateDescribeEipAddressesRequest()
	request.PageSize = requests.NewInteger(pageSize)
	for page := 1; ; page++ {
		request.PageNumber = requests.NewInteger(page)
		response, err := d.client.DescribeEipAddresses(request)
		if err != nil {
			return nil, err
		}

		for _, eip := range response.EipAddresses.EipAddress {
			list.Append(&schema.Resource{
				ID:         d.id,
				Provider:   providerName,
				PublicIPv4: eip.IpAddress,
				Public:     true,
				Service:    d.name(),
			})
		}

		if len(response.EipAddresses.EipAddress) == 0 || page*pageSize >= response.TotalCount {
			break
		}
	}
	return list, nil
}
