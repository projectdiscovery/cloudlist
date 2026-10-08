package azure

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const classicClustersResponse = `{"value": [
  {
    "id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg1/providers/Microsoft.ServiceFabric/clusters/classic1",
    "name": "classic1",
    "type": "Microsoft.ServiceFabric/clusters",
    "location": "eastus",
    "tags": {"env": "prod"},
    "properties": {
      "managementEndpoint": "https://classic1.eastus.cloudapp.azure.com:19080",
      "clusterEndpoint": "https://eastus.servicefabric.azure.com/runtime/clusters/abc",
      "clusterState": "Ready",
      "reliabilityLevel": "Silver",
      "nodeTypes": []
    }
  },
  {
    "id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg1/providers/Microsoft.ServiceFabric/clusters/classic2",
    "name": "classic2",
    "location": "westus",
    "properties": {"managementEndpoint": "https://20.42.0.10:19080", "nodeTypes": []}
  },
  {
    "id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg1/providers/Microsoft.ServiceFabric/clusters/no-endpoint",
    "name": "no-endpoint",
    "location": "westus",
    "properties": {"nodeTypes": []}
  }
]}`

const managedClustersPage1 = `{"value": [
  {
    "id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg2/providers/Microsoft.ServiceFabric/managedClusters/managed1",
    "name": "managed1",
    "location": "eastus",
    "sku": {"name": "Standard"},
    "properties": {
      "dnsName": "managed1",
      "adminUserName": "admin",
      "fqdn": "managed1.eastus.cloudapp.azure.com",
      "ipv4Address": "20.42.0.20",
      "ipv6Address": "2603:1030:20e:3::23c",
      "clientConnectionPort": 19000,
      "httpGatewayConnectionPort": 19080,
      "clusterState": "Ready"
    }
  }
], "nextLink": "%s/subscriptions/00000000-0000-0000-0000-000000000000/providers/Microsoft.ServiceFabric/managedClusters?page=2"}`

const managedClustersPage2 = `{"value": [
  {
    "id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg2/providers/Microsoft.ServiceFabric/managedClusters/managed2",
    "name": "managed2",
    "location": "westus",
    "sku": {"name": "Basic"},
    "properties": {"dnsName": "managed2", "adminUserName": "admin", "fqdn": "managed2.westus.cloudapp.azure.com"}
  }
]}`

// newTestServiceFabricProvider points the real SDK clients at a fake ARM endpoint.
func newTestServiceFabricProvider(t *testing.T, classicStatus, page2Status int) *serviceFabricProvider {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/providers/Microsoft.ServiceFabric/clusters"):
			if classicStatus != http.StatusOK {
				w.WriteHeader(classicStatus)
				_, _ = w.Write([]byte(`{"error": {"code": "AuthorizationFailed", "message": "denied"}}`))
				return
			}
			_, _ = w.Write([]byte(classicClustersResponse))
		case strings.HasSuffix(r.URL.Path, "/providers/Microsoft.ServiceFabric/managedClusters"):
			if r.URL.Query().Get("page") == "2" {
				if page2Status != http.StatusOK {
					w.WriteHeader(page2Status)
					_, _ = w.Write([]byte(`{"error": {"code": "InternalError", "message": "page failed"}}`))
					return
				}
				_, _ = w.Write([]byte(managedClustersPage2))
				return
			}
			_, _ = fmt.Fprintf(w, managedClustersPage1, server.URL)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return &serviceFabricProvider{
		id:               "test",
		SubscriptionID:   testSubscriptionID,
		Credential:       fakeCredential{},
		extendedMetadata: true,
		clientOptions: &arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.Configuration{
					Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
						cloud.ResourceManager: {Endpoint: server.URL, Audience: server.URL},
					},
				},
				Transport: server.Client(),
				Retry:     policy.RetryOptions{MaxRetries: -1},
			},
		},
	}
}

func resourceByValue(resources *schema.Resources) map[string]*schema.Resource {
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

func TestServiceFabricGetResource(t *testing.T) {
	resources, err := newTestServiceFabricProvider(t, http.StatusOK, http.StatusOK).GetResource(context.Background())
	require.NoError(t, err)

	byValue := resourceByValue(resources)
	require.Len(t, byValue, 6, "classic endpoints, managed fqdn/ipv4/ipv6 across both pages")

	classic := byValue["classic1.eastus.cloudapp.azure.com"]
	require.NotNil(t, classic, "classic management endpoint host must be extracted without scheme or port")
	assert.Equal(t, "servicefabric", classic.Service)
	assert.Equal(t, "classic", classic.Metadata["cluster_kind"])
	assert.Equal(t, "rg1", classic.Metadata["resource_group"])
	assert.Equal(t, "Silver", classic.Metadata["reliability_level"])
	assert.Equal(t, "env=prod", classic.Metadata["tags"])

	assert.NotNil(t, byValue["20.42.0.10"], "classic endpoint addressed by IP must be kept")
	assert.Nil(t, byValue["eastus.servicefabric.azure.com"], "the Azure-owned cluster endpoint is not an asset")

	managedIPv4 := byValue["20.42.0.20"]
	require.NotNil(t, managedIPv4)
	assert.True(t, managedIPv4.Public)
	assert.Equal(t, "managed", managedIPv4.Metadata["cluster_kind"])
	assert.Equal(t, "19080", managedIPv4.Metadata["http_gateway_connection_port"])
	assert.Equal(t, "Standard", managedIPv4.Metadata["sku"])
	assert.NotNil(t, byValue["managed1.eastus.cloudapp.azure.com"])
	assert.NotNil(t, byValue["2603:1030:20e:3::23c"])
	assert.NotNil(t, byValue["managed2.westus.cloudapp.azure.com"], "second page must be fetched")
}

func TestServiceFabricGetResourceClassicForbidden(t *testing.T) {
	resources, err := newTestServiceFabricProvider(t, http.StatusForbidden, http.StatusOK).GetResource(context.Background())
	require.NoError(t, err, "a failing classic API must not hide managed clusters")

	byValue := resourceByValue(resources)
	assert.NotNil(t, byValue["managed1.eastus.cloudapp.azure.com"])
	assert.Nil(t, byValue["classic1.eastus.cloudapp.azure.com"])
}

func TestServiceFabricKeepsEarlierManagedPage(t *testing.T) {
	resources, err := newTestServiceFabricProvider(t, http.StatusOK, http.StatusInternalServerError).GetResource(context.Background())
	require.NoError(t, err)

	byValue := resourceByValue(resources)
	assert.NotNil(t, byValue["managed1.eastus.cloudapp.azure.com"])
	assert.NotNil(t, byValue["classic1.eastus.cloudapp.azure.com"])
	assert.Nil(t, byValue["managed2.westus.cloudapp.azure.com"])
}
