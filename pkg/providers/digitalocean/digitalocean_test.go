package digitalocean

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider(t *testing.T, routes map[string]string, services ...string) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if page := r.URL.Query().Get("page"); page != "" && page != "1" {
			key += "?page=" + page
		}
		body, ok := routes[key]
		if !ok {
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := godo.New(http.DefaultClient, godo.SetBaseURL(server.URL+"/"))
	require.NoError(t, err)

	serviceMap := make(schema.ServiceMap)
	for _, s := range services {
		serviceMap[s] = struct{}{}
	}
	return &Provider{id: "test", client: client, services: serviceMap, extendedMetadata: true}
}

func resourcesByValue(resources *schema.Resources) map[string]*schema.Resource {
	byValue := make(map[string]*schema.Resource)
	for _, r := range resources.Items {
		for _, v := range []string{r.DNSName, r.PublicIPv4, r.PublicIPv6} {
			if v != "" {
				byValue[v] = r
			}
		}
	}
	return byValue
}

func TestReservedIPs(t *testing.T) {
	provider := newTestProvider(t, map[string]string{
		"/v2/reserved_ips": `{
			"reserved_ips": [
				{"ip": "45.55.96.47", "region": {"slug": "nyc3"}, "droplet": {"id": 42, "name": "web-1"}, "project_id": "p1"}
			],
			"links": {"pages": {"next": "http://example.com/v2/reserved_ips?page=2", "last": "http://example.com/v2/reserved_ips?page=2"}},
			"meta": {"total": 2}
		}`,
		"/v2/reserved_ips?page=2": `{
			"reserved_ips": [{"ip": "45.55.96.48", "region": {"slug": "nyc3"}, "droplet": null}],
			"links": {"pages": {"first": "http://example.com/v2/reserved_ips?page=1", "prev": "http://example.com/v2/reserved_ips?page=1"}},
			"meta": {"total": 2}
		}`,
		"/v2/reserved_ipv6": `{
			"reserved_ipv6s": [{"ip": "2604:a880:800:14::42c3:d000", "region_slug": "nyc3", "reserved_at": "2024-01-01T00:00:00Z"}],
			"links": {},
			"meta": {"total": 1}
		}`,
	}, "reservedip")

	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)
	byValue := resourcesByValue(resources)
	require.Len(t, byValue, 3)

	attached := byValue["45.55.96.47"]
	require.NotNil(t, attached)
	assert.Equal(t, "reservedip", attached.Service)
	assert.Equal(t, "42", attached.Metadata["droplet_id"])

	unassigned := byValue["45.55.96.48"]
	require.NotNil(t, unassigned, "second page must be fetched")
	assert.NotContains(t, unassigned.Metadata, "droplet_id")

	ipv6 := byValue["2604:a880:800:14::42c3:d000"]
	require.NotNil(t, ipv6)
	assert.Equal(t, "nyc3", ipv6.Metadata["region_slug"])
	assert.Equal(t, "2024-01-01T00:00:00Z", ipv6.Metadata["reserved_at"])
}

func TestLoadBalancers(t *testing.T) {
	provider := newTestProvider(t, map[string]string{
		"/v2/load_balancers": `{
			"load_balancers": [
				{"id": "lb-1", "name": "regional", "ip": "104.131.186.241", "ipv6": "2604:a880:400:d1::90c:6001", "type": "REGIONAL", "region": {"slug": "nyc1"}, "droplet_ids": [1, 2]},
				{"id": "lb-2", "name": "global", "type": "GLOBAL", "domains": [{"name": "app.example.com", "is_managed": true}]}
			],
			"links": {},
			"meta": {"total": 2}
		}`,
	}, "loadbalancer")

	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)
	byValue := resourcesByValue(resources)
	require.Len(t, byValue, 3)

	regional := byValue["104.131.186.241"]
	require.NotNil(t, regional)
	assert.Equal(t, "loadbalancer", regional.Service)
	assert.Equal(t, "2", regional.Metadata["droplet_count"])
	require.NotNil(t, byValue["2604:a880:400:d1::90c:6001"])

	global := byValue["app.example.com"]
	require.NotNil(t, global)
	assert.Equal(t, "GLOBAL", global.Metadata["type"])
}

func TestKubernetesClusters(t *testing.T) {
	provider := newTestProvider(t, map[string]string{
		"/v2/kubernetes/clusters": `{
			"kubernetes_clusters": [
				{"id": "bd5f5959", "name": "prod", "region": "nyc1", "version": "1.31.1-do.0", "ipv4": "68.183.121.157", "endpoint": "https://bd5f5959.k8s.ondigitalocean.com", "status": {"state": "running"}}
			],
			"links": {},
			"meta": {"total": 1}
		}`,
	}, "kubernetes")

	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)
	byValue := resourcesByValue(resources)
	require.Len(t, byValue, 2)

	host := byValue["bd5f5959.k8s.ondigitalocean.com"]
	require.NotNil(t, host)
	assert.Equal(t, "kubernetes", host.Service)
	assert.Equal(t, "running", host.Metadata["status"])
	require.NotNil(t, byValue["68.183.121.157"])
}

func TestResourcesSkipsUnreadableNewServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/load_balancers" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"load_balancers": [{"id": "lb-1", "ip": "159.203.150.1"}], "links": {}, "meta": {"total": 1}}`))
			return
		}
		// Simulates a custom-scoped token without reserved_ip:read or kubernetes:read.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"id": "forbidden", "message": "You are not authorized to perform this operation"}`))
	}))
	t.Cleanup(server.Close)

	client, err := godo.New(http.DefaultClient, godo.SetBaseURL(server.URL+"/"))
	require.NoError(t, err)
	provider := &Provider{id: "test", client: client, services: schema.ServiceMap{
		"reservedip": struct{}{}, "loadbalancer": struct{}{}, "kubernetes": struct{}{},
	}}

	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)
	assert.Contains(t, resourcesByValue(resources), "159.203.150.1")
}

func TestReservedIPv4KeptWhenIPv6Fails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/reserved_ips" {
			_, _ = w.Write([]byte(`{"reserved_ips":[{"ip":"45.55.96.47"}],"links":{},"meta":{"total":1}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"id":"forbidden","message":"denied"}`))
	}))
	t.Cleanup(server.Close)

	client, err := godo.New(http.DefaultClient, godo.SetBaseURL(server.URL+"/"))
	require.NoError(t, err)
	provider := &Provider{id: "test", client: client, services: schema.ServiceMap{"reservedip": {}}}

	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)
	assert.Contains(t, resourcesByValue(resources), "45.55.96.47")
}
