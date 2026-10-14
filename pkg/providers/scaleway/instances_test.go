package scaleway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstancesSecondPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"servers":[{"id":"b","public_ip":{"address":"51.15.0.2"}}],"total_count":2}`))
			return
		}
		_, _ = w.Write([]byte(`{"servers":[{"id":"a","public_ip":{"address":"51.15.0.1"}}],"total_count":2}`))
	}))
	t.Cleanup(server.Close)

	client, err := scw.NewClient(scw.WithAuth("SCWXXXXXXXXXXXXXXXXX", "00000000-0000-0000-0000-000000000000"), scw.WithAPIURL(server.URL), scw.WithHTTPClient(server.Client()))
	require.NoError(t, err)

	require.NotPanics(t, func() {
		list, err := (&instanceProvider{id: "test", instanceAPI: instance.NewAPI(client)}).GetResource(context.Background())
		require.NoError(t, err)
		var ips []string
		for _, r := range list.Items {
			ips = append(ips, r.PublicIPv4)
		}
		assert.ElementsMatch(t, []string{"51.15.0.1", "51.15.0.2"}, ips)
	})
}
