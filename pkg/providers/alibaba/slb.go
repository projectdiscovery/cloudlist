package alibaba

import (
	"context"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// slbProvider is a Classic Load Balancer (SLB) provider for alibaba API
type slbProvider struct {
	id     string
	client *slb.Client
}

func (d *slbProvider) name() string {
	return "slb"
}

// GetResource returns all the resources in the store for a provider.
func (d *slbProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	request := slb.CreateDescribeLoadBalancersRequest()
	request.PageSize = requests.NewInteger(pageSize)
	for page := 1; ; page++ {
		request.PageNumber = requests.NewInteger(page)
		response, err := d.client.DescribeLoadBalancers(request)
		if err != nil {
			return list, err
		}

		for _, lb := range response.LoadBalancers.LoadBalancer {
			resource := &schema.Resource{
				ID:       d.id,
				Provider: providerName,
				Service:  d.name(),
			}
			if lb.AddressType == "internet" {
				resource.Public = true
				resource.PublicIPv4 = lb.Address
			} else {
				resource.PrivateIpv4 = lb.Address
			}
			list.Append(resource)
		}

		if len(response.LoadBalancers.LoadBalancer) == 0 || page*pageSize >= response.TotalCount {
			break
		}
	}
	return list, nil
}
