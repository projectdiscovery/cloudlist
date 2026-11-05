package dnssimple

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dnsimple/dnsimple-go/dnsimple"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider(t *testing.T, routes map[string]string) *Provider {
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

	client := dnsimple.NewClient(http.DefaultClient)
	client.BaseURL = server.URL
	return &Provider{id: "test", client: client, account: "1010", services: schema.ServiceMap{"dns": struct{}{}}}
}

func TestResourcesPaginates(t *testing.T) {
	provider := newTestProvider(t, map[string]string{
		"/v2/1010/domains": `{
			"data": [{"id": 1, "name": "example.com"}],
			"pagination": {"current_page": 1, "per_page": 1, "total_entries": 2, "total_pages": 2}
		}`,
		"/v2/1010/domains?page=2": `{
			"data": [{"id": 2, "name": "example.org"}],
			"pagination": {"current_page": 2, "per_page": 1, "total_entries": 2, "total_pages": 2}
		}`,
		"/v2/1010/zones/example.com/records": `{
			"data": [{"id": 10, "zone_id": "example.com", "name": "www", "type": "A", "content": "192.0.2.1"}],
			"pagination": {"current_page": 1, "per_page": 1, "total_entries": 2, "total_pages": 2}
		}`,
		"/v2/1010/zones/example.com/records?page=2": `{
			"data": [{"id": 11, "zone_id": "example.com", "name": "api", "type": "AAAA", "content": "2001:db8::1"}],
			"pagination": {"current_page": 2, "per_page": 1, "total_entries": 2, "total_pages": 2}
		}`,
		"/v2/1010/zones/example.org/records": `{
			"data": [{"id": 20, "zone_id": "example.org", "name": "", "type": "A", "content": "192.0.2.2"}],
			"pagination": {"current_page": 1, "per_page": 1, "total_entries": 1, "total_pages": 1}
		}`,
	})

	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)

	got := make(map[string]string)
	for _, r := range resources.Items {
		got[r.DNSName] = r.PublicIPv4 + r.PublicIPv6
	}
	assert.Equal(t, map[string]string{
		"www.example.com": "192.0.2.1",
		"api.example.com": "2001:db8::1",
		"example.org":     "192.0.2.2",
	}, got)
}
