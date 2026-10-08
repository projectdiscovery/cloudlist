package openstack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServiceClient(t *testing.T, path, body string) *gophercloud.ServiceClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, path, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{TokenID: "test"},
		Endpoint:       server.URL + "/",
		ResourceBase:   server.URL + "/v2.0/",
	}
}

func TestFloatingIPProvider(t *testing.T) {
	client := newTestServiceClient(t, "/v2.0/floatingips", `{"floatingips": [
		{"id": "attached", "floating_ip_address": "185.12.64.10", "port_id": "port-1", "fixed_ip_address": "10.0.0.5"},
		{"id": "unattached", "floating_ip_address": "185.12.64.20", "port_id": null}
	]}`)

	resources, err := (&floatingIPProvider{id: "test", client: client}).GetResource(context.Background())
	require.NoError(t, err)
	require.Len(t, resources.Items, 2)

	for i, ip := range []string{"185.12.64.10", "185.12.64.20"} {
		assert.Equal(t, ip, resources.Items[i].PublicIPv4)
		assert.True(t, resources.Items[i].Public)
		assert.Equal(t, "floatingip", resources.Items[i].Service)
	}
}

func TestLoadBalancerProvider(t *testing.T) {
	client := newTestServiceClient(t, "/v2.0/lbaas/loadbalancers", `{"loadbalancers": [
		{"id": "private", "name": "internal", "vip_address": "10.0.0.20"},
		{"id": "public", "name": "edge", "vip_address": "185.12.64.30"}
	]}`)

	resources, err := (&loadBalancerProvider{id: "test", client: client}).GetResource(context.Background())
	require.NoError(t, err)
	require.Len(t, resources.Items, 2)

	assert.Equal(t, "10.0.0.20", resources.Items[0].PrivateIpv4)
	assert.False(t, resources.Items[0].Public)
	assert.Equal(t, "185.12.64.30", resources.Items[1].PublicIPv4)
	assert.True(t, resources.Items[1].Public)
	assert.Equal(t, "loadbalancer", resources.Items[1].Service)
}

func TestOptionalClientDropsClientOnError(t *testing.T) {
	assert.Nil(t, optionalClient(&gophercloud.ServiceClient{}, &gophercloud.ErrEndpointNotFound{}))
	assert.NotNil(t, optionalClient(&gophercloud.ServiceClient{}, nil))
}

func TestFloatingIPProviderKeepsEarlierPage(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("marker") == "2" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"page failed"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"floatingips":[{"id":"one","floating_ip_address":"185.12.64.10"}],"floatingips_links":[{"href":"` + server.URL + `/v2.0/floatingips?marker=2","rel":"next"}]}`))
	}))
	t.Cleanup(server.Close)

	client := &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{TokenID: "test"},
		Endpoint:       server.URL + "/",
		ResourceBase:   server.URL + "/v2.0/",
	}
	provider := &Provider{id: "test", network: client, services: schema.ServiceMap{"floatingip": {}}}
	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)
	require.Len(t, resources.Items, 1)
	assert.Equal(t, "185.12.64.10", resources.Items[0].PublicIPv4)
}
