package alibaba

import (
	"context"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for alibaba API
type instanceProvider struct {
	id     string
	client *ecs.Client
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetResource returns all the resources in the store for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	request := ecs.CreateDescribeInstancesRequest()

	response, err := d.client.DescribeInstances(request)
	if err != nil {
		return nil, err
	}

	for _, instance := range response.Instances.Instance {

		var ipv4, privateIPv4 string
		if len(instance.PublicIpAddress.IpAddress) > 0 {
			ipv4 = instance.PublicIpAddress.IpAddress[0]
		}
		if len(instance.NetworkInterfaces.NetworkInterface) > 0 && len(instance.NetworkInterfaces.NetworkInterface[0].PrivateIpSets.PrivateIpSet) > 0 {
			privateIPv4 = instance.NetworkInterfaces.NetworkInterface[0].PrivateIpSets.PrivateIpSet[0].PrivateIpAddress
		}
		list.Append(&schema.Resource{
			ID:          d.id,
			Provider:    providerName,
			PublicIPv4:  ipv4,
			PrivateIpv4: privateIPv4,
			Public:      ipv4 != "",
			Service:     d.name(),
		})
	}

	return list, nil
}
