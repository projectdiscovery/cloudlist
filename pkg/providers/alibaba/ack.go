package alibaba

import (
	"context"
	"encoding/json"
	"net"
	"net/url"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cs"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// ackProvider is a Container Service for Kubernetes (ACK) provider for alibaba API
type ackProvider struct {
	id     string
	client *cs.Client
}

// The cs SDK package leaves ROA response bodies untyped, so the fields we
// need are decoded here.
type ackClustersResponse struct {
	Clusters []struct {
		// MasterURL is itself a JSON-encoded object.
		MasterURL string `json:"master_url"`
	} `json:"clusters"`
	PageInfo struct {
		TotalCount int `json:"total_count"`
	} `json:"page_info"`
}

type ackMasterURL struct {
	APIServerEndpoint         string `json:"api_server_endpoint"`
	IntranetAPIServerEndpoint string `json:"intranet_api_server_endpoint"`
}

func (d *ackProvider) name() string {
	return "ack"
}

// GetResource returns all the resources in the store for a provider.
func (d *ackProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	request := cs.CreateDescribeClustersV1Request()
	request.PageSize = requests.NewInteger(pageSize)
	for page := 1; ; page++ {
		request.PageNumber = requests.NewInteger(page)
		response, err := d.client.DescribeClustersV1(request)
		if err != nil {
			return list, err
		}

		var clusters ackClustersResponse
		if err := json.Unmarshal(response.GetHttpContentBytes(), &clusters); err != nil {
			return list, err
		}

		for _, cluster := range clusters.Clusters {
			var masterURL ackMasterURL
			if err := json.Unmarshal([]byte(cluster.MasterURL), &masterURL); err != nil {
				continue
			}
			d.appendEndpoint(list, masterURL.APIServerEndpoint, true)
			d.appendEndpoint(list, masterURL.IntranetAPIServerEndpoint, false)
		}

		if len(clusters.Clusters) == 0 || page*pageSize >= clusters.PageInfo.TotalCount {
			break
		}
	}
	return list, nil
}

// appendEndpoint adds the host of an API server URL such as https://47.0.0.1:6443.
func (d *ackProvider) appendEndpoint(list *schema.Resources, endpoint string, public bool) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return
	}
	host := parsed.Hostname()

	resource := &schema.Resource{
		ID:       d.id,
		Provider: providerName,
		Public:   public,
		Service:  d.name(),
	}
	switch {
	case net.ParseIP(host) == nil:
		resource.DNSName = host
	case public:
		resource.PublicIPv4 = host
	default:
		resource.PrivateIpv4 = host
	}
	list.Append(resource)
}
