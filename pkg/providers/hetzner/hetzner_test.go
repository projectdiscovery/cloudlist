package hetzner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	hetzner "github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fakeResponses = map[string]string{
	"/servers": `{"servers": []}`,
	"/load_balancers": `{"load_balancers": [
		{"id": 1, "name": "public-lb", "public_net": {"enabled": true,
			"ipv4": {"ip": "95.217.10.1"}, "ipv6": {"ip": "2a01:4f9:c010:1::1"}}},
		{"id": 2, "name": "private-lb", "public_net": {"enabled": false,
			"ipv4": {"ip": "95.217.10.2"}, "ipv6": {"ip": ""}}}
	]}`,
	"/floating_ips": `{"floating_ips": [
		{"id": 1, "type": "ipv4", "ip": "95.217.20.1", "server": 42},
		{"id": 2, "type": "ipv6", "ip": "2a01:4f9:c010:2::/64", "server": null}
	]}`,
	"/primary_ips": `{"primary_ips": [
		{"id": 1, "type": "ipv4", "ip": "95.217.30.1", "assignee_id": 42, "assignee_type": "server"},
		{"id": 2, "type": "ipv4", "ip": "95.217.30.2", "assignee_id": null, "assignee_type": "server"}
	]}`,
}

func newTestProvider(t *testing.T, services ...string) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := fakeResponses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client := hetzner.NewClient(hetzner.WithToken("test"), hetzner.WithEndpoint(server.URL))
	block := schema.OptionBlock{}
	if len(services) > 0 {
		block["services"] = services[0]
	}
	return &Provider{id: "test", client: client, services: block.ResolveServices(Services)}
}

func resourcesByService(resources *schema.Resources) map[string][]string {
	byService := make(map[string][]string)
	for _, r := range resources.Items {
		for _, v := range []string{r.PublicIPv4, r.PublicIPv6} {
			if v != "" {
				byService[r.Service] = append(byService[r.Service], v)
			}
		}
	}
	return byService
}

func TestResources(t *testing.T) {
	resources, err := newTestProvider(t).Resources(context.Background())
	require.NoError(t, err)

	byService := resourcesByService(resources)
	assert.ElementsMatch(t, []string{"95.217.10.1", "2a01:4f9:c010:1::1"}, byService["loadbalancer"], "load balancers with public net disabled must be skipped")
	assert.ElementsMatch(t, []string{"95.217.20.1", "2a01:4f9:c010:2::"}, byService["floatingip"])
	assert.ElementsMatch(t, []string{"95.217.30.1", "95.217.30.2"}, byService["primaryip"], "unassigned primary IPs must be included")
}

func TestResourcesServiceFilter(t *testing.T) {
	resources, err := newTestProvider(t, "primaryip").Resources(context.Background())
	require.NoError(t, err)

	byService := resourcesByService(resources)
	assert.Len(t, byService, 1)
	assert.Len(t, byService["primaryip"], 2)
}
