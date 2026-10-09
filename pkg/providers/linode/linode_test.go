package linode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linode/linodego"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func page(data string) string {
	return fmt.Sprintf(`{"data": %s, "page": 1, "pages": 1, "results": 1}`, data)
}

var linodeAPI = map[string]string{
	"/v4/linode/instances": page(`[{"id": 1, "ipv4": ["45.79.10.1"]}]`),
	"/v4/nodebalancers":    page(`[{"id": 2, "hostname": "nb-45-79-10-2.newark.nodebalancer.linode.com", "ipv4": "45.79.10.2", "ipv6": "2600:3c00:1::2"}]`),
	"/v4/networking/ips": page(`[
		{"address": "45.79.10.1", "type": "ipv4", "public": true, "linode_id": 1},
		{"address": "45.79.10.3", "type": "ipv4", "public": true},
		{"address": "192.168.128.5", "type": "ipv4", "public": false, "linode_id": 1}
	]`),
	"/v4/lke/clusters":                 page(`[{"id": 3, "label": "prod"}, {"id": 4, "label": "provisioning"}]`),
	"/v4/lke/clusters/3/api-endpoints": page(`[{"endpoint": "https://abc123.us-east-1.linodelke.net:443"}]`),
}

func newTestProvider(t *testing.T, services ...string) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := linodeAPI[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"errors": [{"reason": "not ready"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client := linodego.NewClient(server.Client())
	client.SetBaseURL(server.URL)
	client.SetRetryCount(0)

	block := schema.OptionBlock{}
	if len(services) > 0 {
		block["services"] = services[0]
	}
	return &Provider{id: "test", client: &client, services: block.ResolveServices(Services)}
}

func TestResources(t *testing.T) {
	resources, err := newTestProvider(t).Resources(context.Background())
	require.NoError(t, err)

	got := map[string]string{}
	for _, r := range resources.Items {
		value := r.DNSName + r.PublicIPv4 + r.PublicIPv6 + r.PrivateIpv4
		got[value] = r.Service
	}

	assert.Equal(t, map[string]string{
		"45.79.10.1": "instance",
		"nb-45-79-10-2.newark.nodebalancer.linode.com": "nodebalancer",
		"45.79.10.2":                     "nodebalancer",
		"2600:3c00:1::2":                 "nodebalancer",
		"45.79.10.3":                     "ip",
		"abc123.us-east-1.linodelke.net": "lke",
	}, got)
}

func TestResourcesServiceFilter(t *testing.T) {
	resources, err := newTestProvider(t, "lke").Resources(context.Background())
	require.NoError(t, err)
	require.Len(t, resources.Items, 1)
	assert.Equal(t, "abc123.us-east-1.linodelke.net", resources.Items[0].DNSName)
}
