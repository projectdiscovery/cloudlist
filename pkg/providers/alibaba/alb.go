package alibaba

import (
	"context"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// albProvider is an Application Load Balancer provider for alibaba API
type albProvider struct {
	id     string
	client *alb.Client
}

func (d *albProvider) name() string {
	return "alb"
}

// GetResource returns all the resources in the store for a provider.
func (d *albProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	request := alb.CreateListLoadBalancersRequest()
	request.MaxResults = requests.NewInteger(pageSize)
	for {
		response, err := d.client.ListLoadBalancers(request)
		if err != nil {
			return list, err
		}

		for _, lb := range response.LoadBalancers {
			list.Append(&schema.Resource{
				ID:       d.id,
				Provider: providerName,
				DNSName:  lb.DNSName,
				Public:   lb.AddressType == "Internet",
				Service:  d.name(),
			})
		}

		if response.NextToken == "" {
			break
		}
		request.NextToken = response.NextToken
	}
	return list, nil
}
