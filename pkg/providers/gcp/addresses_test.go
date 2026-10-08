package gcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

var fakeComputeResponses = map[string]string{
	"/projects/p1/aggregated/instances": `{"items":{"zones/us-central1-a":{"instances":[
			{"name":"vm-1","networkInterfaces":[{"accessConfigs":[{"natIP":"34.10.0.1"}]}]}]}}}`,
	"/projects/p1/aggregated/addresses": `{"items":{
			"regions/us-central1":{"addresses":[
				{"name":"vm-1-ip","address":"34.10.0.1","addressType":"EXTERNAL","status":"IN_USE"},
				{"name":"reserved","address":"34.10.0.2","addressType":"EXTERNAL","status":"RESERVED","region":"https://www.googleapis.com/compute/v1/projects/p1/regions/us-central1"},
				{"name":"internal","address":"10.128.0.5","addressType":"INTERNAL","status":"RESERVED"}]},
			"regions/europe-west1":{"warning":{"code":"NO_RESULTS_ON_PAGE"}}}}`,
	"/projects/p1/global/addresses": `{"items":[{"name":"global-ip","address":"34.120.0.3","addressType":"EXTERNAL"}]}`,
	"/projects/p1/aggregated/forwardingRules": `{"items":{"regions/us-central1":{"forwardingRules":[
			{"name":"nlb","IPAddress":"35.200.0.4","loadBalancingScheme":"EXTERNAL","IPProtocol":"TCP","portRange":"443-443","target":"https://www.googleapis.com/compute/v1/projects/p1/regions/us-central1/targetPools/pool-1"}]}}}`,
	"/projects/p1/global/forwardingRules": `{"items":[{"name":"https-lb","IPAddress":"34.120.0.3","loadBalancingScheme":"EXTERNAL_MANAGED"}]}`,
}

func newFakeComputeService(t *testing.T, responses map[string]string) *compute.Service {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[strings.TrimPrefix(r.URL.Path, "/compute/v1")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	svc, err := compute.NewService(context.Background(),
		option.WithEndpoint(server.URL+"/compute/v1/"),
		option.WithoutAuthentication(),
		option.WithHTTPClient(server.Client()),
	)
	require.NoError(t, err)
	return svc
}

func TestCloudVMProvider_IncludesAddressesAndForwardingRules(t *testing.T) {
	provider := &cloudVMProvider{
		id:               "test",
		compute:          newFakeComputeService(t, fakeComputeResponses),
		projects:         []string{"p1"},
		extendedMetadata: true,
	}

	resources, err := provider.GetResource(context.Background())
	require.NoError(t, err)

	byIP := make(map[string]map[string]string)
	var private []string
	for _, r := range resources.Items {
		switch {
		case r.PublicIPv4 != "":
			byIP[r.PublicIPv4] = r.Metadata
		case r.PrivateIpv4 != "":
			private = append(private, r.PrivateIpv4)
		}
	}

	assert.Len(t, byIP, 4)
	assert.Equal(t, "vm-1", byIP["34.10.0.1"]["instance_name"], "in-use address must keep the instance entry")
	assert.Equal(t, "RESERVED", byIP["34.10.0.2"]["status"])
	assert.Equal(t, "us-central1", byIP["34.10.0.2"]["region"])
	assert.Equal(t, "global-ip", byIP["34.120.0.3"]["address_name"], "global address and global forwarding rule share an IP")
	assert.Equal(t, "nlb", byIP["35.200.0.4"]["forwarding_rule_name"])
	assert.Equal(t, "pool-1", byIP["35.200.0.4"]["target"])
	assert.Equal(t, []string{"10.128.0.5"}, private, "internal addresses are classified as private")
}
