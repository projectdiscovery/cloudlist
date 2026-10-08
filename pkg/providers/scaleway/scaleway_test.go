package scaleway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestScalewayAPI(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		query := r.URL.Query()
		switch r.URL.Path {
		case "/instance/v1/zones/fr-par-1/ips":
			// Two pages, to check every page is fetched.
			if query.Get("page") == "2" {
				_, _ = w.Write([]byte(`{"ips":[{"address":"51.15.0.11"}],"total_count":2}`))
				return
			}
			_, _ = w.Write([]byte(`{"ips":[{"address":"51.15.0.10"}],"total_count":2}`))
		case "/flexible-ip/v1alpha1/zones/fr-par-2/fips":
			_, _ = w.Write([]byte(`{"flexible_ips":[{"ip_address":"51.159.0.20/32"}],"total_count":1}`))
		case "/lb/v1/zones/nl-ams-1/ips":
			// A failing zone must not hide results from the others.
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		case "/lb/v1/zones/fr-par-1/ips":
			_, _ = w.Write([]byte(`{"ips":[{"ip_address":"51.159.10.30"}],"total_count":1}`))
		case "/containers/v1beta1/regions/fr-par/namespaces", "/functions/v1beta1/regions/fr-par/namespaces":
			_, _ = w.Write([]byte(`{"namespaces":[{"id":"ns-1"}],"total_count":1}`))
		case "/containers/v1beta1/regions/fr-par/containers":
			assert.Equal(t, "ns-1", query.Get("namespace_id"))
			_, _ = w.Write([]byte(`{"containers":[{"domain_name":"ns1-app.functions.fnc.fr-par.scw.cloud"}],"total_count":1}`))
		case "/functions/v1beta1/regions/fr-par/functions":
			assert.Equal(t, "ns-1", query.Get("namespace_id"))
			_, _ = w.Write([]byte(`{"functions":[{"domain_name":"ns1-fn.functions.fnc.fr-par.scw.cloud"}],"total_count":1}`))
		case "/k8s/v1/regions/nl-ams/clusters":
			_, _ = w.Write([]byte(`{"clusters":[{"cluster_url":"https://abc.api.k8s.nl-ams.scw.cloud:6443"}],"total_count":1}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestResources(t *testing.T) {
	server := newTestScalewayAPI(t)
	client, err := scw.NewClient(
		scw.WithAuth("SCWXXXXXXXXXXXXXXXXX", "11111111-1111-1111-1111-111111111111"),
		scw.WithAPIURL(server.URL),
	)
	require.NoError(t, err)

	services := schema.ServiceMap{}
	for _, s := range Services {
		services[s] = struct{}{}
	}
	provider := &Provider{id: "test", client: client, services: services}

	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)

	var got []string
	for _, r := range resources.Items {
		got = append(got, r.Service+" "+r.PublicIPv4+r.DNSName)
		assert.True(t, r.Public)
	}
	sort.Strings(got)
	assert.Equal(t, []string{
		"container ns1-app.functions.fnc.fr-par.scw.cloud",
		"flexibleip 51.15.0.10",
		"flexibleip 51.15.0.11",
		"flexibleip 51.159.0.20",
		"function ns1-fn.functions.fnc.fr-par.scw.cloud",
		"kapsule abc.api.k8s.nl-ams.scw.cloud",
		"lb 51.159.10.30",
	}, got)
}

func TestFlexibleIPsKeptWhenMetalListingFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/flexible-ip/") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"failed"}`))
			return
		}
		if strings.Contains(r.URL.Path, "/instance/") && strings.HasSuffix(r.URL.Path, "/ips") {
			_, _ = w.Write([]byte(`{"ips":[{"address":"51.15.0.10"}],"total_count":1}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	client, err := scw.NewClient(
		scw.WithAuth("SCWXXXXXXXXXXXXXXXXX", "11111111-1111-1111-1111-111111111111"),
		scw.WithAPIURL(server.URL),
	)
	require.NoError(t, err)

	provider := &Provider{id: "test", client: client, services: schema.ServiceMap{"flexibleip": {}}}
	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)

	var got []string
	for _, item := range resources.Items {
		got = append(got, item.PublicIPv4)
	}
	assert.Contains(t, got, "51.15.0.10")
}
