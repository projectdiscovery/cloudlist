package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSubscriptionID = "00000000-0000-0000-0000-000000000000"

var fakeARMResponses = map[string]string{
	"/subscriptions/" + testSubscriptionID + "/providers/Microsoft.Cache/redis": `{"value":[
		{"id":"/subscriptions/` + testSubscriptionID + `/resourceGroups/rg-cache/providers/Microsoft.Cache/Redis/contoso-cache","name":"contoso-cache","location":"eastus",
		 "tags":{"env":"prod"},
		 "properties":{"hostName":"contoso-cache.redis.cache.windows.net","port":6379,"sslPort":6380,"enableNonSslPort":false,"publicNetworkAccess":"Disabled","redisVersion":"6.0","provisioningState":"Succeeded"}},
		{"id":"/subscriptions/` + testSubscriptionID + `/resourceGroups/rg-cache/providers/Microsoft.Cache/Redis/provisioning","name":"provisioning","properties":{}}
	]}`,
	"/subscriptions/" + testSubscriptionID + "/providers/Microsoft.DBforMySQL/flexibleServers": `{"value":[
		{"id":"/subscriptions/` + testSubscriptionID + `/resourceGroups/rg-db/providers/Microsoft.DBforMySQL/flexibleServers/contoso-mysql","name":"contoso-mysql","location":"westeurope",
		 "sku":{"name":"Standard_B1ms","tier":"Burstable"},
		 "properties":{"fullyQualifiedDomainName":"contoso-mysql.mysql.database.azure.com","version":"8.0.21","state":"Ready","network":{"publicNetworkAccess":"Enabled"}}}
	]}`,
	"/subscriptions/" + testSubscriptionID + "/providers/Microsoft.DBforPostgreSQL/flexibleServers": `{"value":[
		{"id":"/subscriptions/` + testSubscriptionID + `/resourceGroups/rg-db/providers/Microsoft.DBforPostgreSQL/flexibleServers/contoso-pg","name":"contoso-pg","location":"westeurope",
		 "properties":{"fullyQualifiedDomainName":"contoso-pg.postgres.database.azure.com","version":"16","state":"Ready","network":{"publicNetworkAccess":"Disabled"}}}
	]}`,
	"/subscriptions/" + testSubscriptionID + "/providers/Microsoft.Sql/servers": `{"value":[
		{"id":"/subscriptions/` + testSubscriptionID + `/resourceGroups/rg-db/providers/Microsoft.Sql/servers/contoso-sql","name":"contoso-sql","location":"eastus",
		 "properties":{"fullyQualifiedDomainName":"contoso-sql.database.windows.net","version":"12.0","state":"Ready","publicNetworkAccess":"Enabled","minimalTlsVersion":"1.2"}}
	]}`,
	"/subscriptions/" + testSubscriptionID + "/providers/Microsoft.DocumentDB/databaseAccounts": `{"value":[
		{"id":"/subscriptions/` + testSubscriptionID + `/resourceGroups/rg-db/providers/Microsoft.DocumentDB/databaseAccounts/contoso-cosmos","name":"contoso-cosmos","location":"eastus","kind":"GlobalDocumentDB",
		 "properties":{"documentEndpoint":"https://contoso-cosmos.documents.azure.com:443/","publicNetworkAccess":"Enabled",
		   "writeLocations":[{"locationName":"East US","documentEndpoint":"https://contoso-cosmos-eastus.documents.azure.com:443/"}],
		   "readLocations":[{"locationName":"East US","documentEndpoint":"https://contoso-cosmos-eastus.documents.azure.com:443/"},{"locationName":"West US","documentEndpoint":"https://contoso-cosmos-westus.documents.azure.com:443/"}]}},
		{"id":"/subscriptions/` + testSubscriptionID + `/resourceGroups/rg-db/providers/Microsoft.DocumentDB/databaseAccounts/contoso-mongo","name":"contoso-mongo","location":"eastus","kind":"MongoDB",
		 "properties":{"documentEndpoint":"https://contoso-mongo.documents.azure.com:443/","publicNetworkAccess":"Enabled",
		   "apiProperties":{"serverVersion":"4.2"},
		   "writeLocations":[{"locationName":"East US","documentEndpoint":"https://contoso-mongo-eastus.documents.azure.com:443/"}]}},
		{"id":"/subscriptions/` + testSubscriptionID + `/resourceGroups/rg-db/providers/Microsoft.DocumentDB/databaseAccounts/contoso-mongo32","name":"contoso-mongo32","location":"eastus","kind":"MongoDB",
		 "properties":{"documentEndpoint":"https://contoso-mongo32.documents.azure.com:443/","publicNetworkAccess":"Enabled",
		   "apiProperties":{"serverVersion":"3.2"},
		   "writeLocations":[{"locationName":"East US","documentEndpoint":"https://contoso-mongo32-eastus.documents.azure.com:443/"}]}}
	]}`,
}

type fakeCredential struct{}

func (fakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// newTestDatabaseProvider points the real ARM clients at a fake endpoint.
// TLS is required because the SDK refuses to send bearer tokens over plain HTTP.
func newTestDatabaseProvider(t *testing.T) *databaseProvider {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := fakeARMResponses[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return &databaseProvider{
		id:               "test",
		SubscriptionID:   testSubscriptionID,
		Credential:       fakeCredential{},
		extendedMetadata: true,
		clientOptions: &arm.ClientOptions{ClientOptions: policy.ClientOptions{
			Cloud: cloud.Configuration{Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
				cloud.ResourceManager: {Endpoint: server.URL, Audience: "https://management.azure.com"},
			}},
			Transport: server.Client(),
		}},
	}
}

func TestDatabaseProviderFetchers(t *testing.T) {
	dp := newTestDatabaseProvider(t)

	tests := []struct {
		service  string
		hosts    []string
		metadata map[string]string
	}{
		{
			service: "redis",
			hosts:   []string{"contoso-cache.redis.cache.windows.net"},
			metadata: map[string]string{
				"public_network_access": "Disabled", "resource_group": "rg-cache", "ssl_port": "6380",
				"non_ssl_port_enabled": "false", "tags": "env=prod",
			},
		},
		{
			service:  "mysql",
			hosts:    []string{"contoso-mysql.mysql.database.azure.com"},
			metadata: map[string]string{"public_network_access": "Enabled", "sku_name": "Standard_B1ms", "version": "8.0.21"},
		},
		{
			service:  "postgresql",
			hosts:    []string{"contoso-pg.postgres.database.azure.com"},
			metadata: map[string]string{"public_network_access": "Disabled", "version": "16"},
		},
		{
			service:  "sql",
			hosts:    []string{"contoso-sql.database.windows.net"},
			metadata: map[string]string{"public_network_access": "Enabled", "minimal_tls_version": "1.2"},
		},
		{
			service: "cosmosdb",
			hosts: []string{
				"contoso-cosmos.documents.azure.com",
				"contoso-cosmos-eastus.documents.azure.com",
				"contoso-cosmos-westus.documents.azure.com",
				"contoso-mongo.documents.azure.com",
				"contoso-mongo.mongo.cosmos.azure.com",
				"contoso-mongo-eastus.documents.azure.com",
				"contoso-mongo-eastus.mongo.cosmos.azure.com",
				"contoso-mongo32.documents.azure.com",
				"contoso-mongo32-eastus.documents.azure.com",
			},
			metadata: map[string]string{"public_network_access": "Enabled"},
		},
	}

	fetchers := dp.fetchers()
	require.Len(t, fetchers, len(tests))
	for i, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			require.Equal(t, tt.service, fetchers[i].service)

			resources, err := fetchers[i].fetch(context.Background())
			require.NoError(t, err)

			var hosts []string
			for _, resource := range resources.Items {
				assert.Equal(t, tt.service, resource.Service)
				assert.Equal(t, providerName, resource.Provider)
				for key, value := range tt.metadata {
					assert.Equal(t, value, resource.Metadata[key], key)
				}
				hosts = append(hosts, resource.DNSName)
			}
			assert.Equal(t, tt.hosts, hosts)
		})
	}
}
