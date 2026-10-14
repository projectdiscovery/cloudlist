package linode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceIPv6(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The API returns the SLAAC address with its prefix length.
		_, _ = w.Write([]byte(`{"data":[{"id":1,"label":"web","ipv4":["172.105.10.20"],"ipv6":"2600:3c03::f03c:91ff:fe24:3a2f/128"}],"page":1,"pages":1,"results":1}`))
	}))
	t.Cleanup(server.Close)

	client := linodego.NewClient(server.Client())
	client.SetBaseURL(server.URL)

	resources, err := (&instanceProvider{id: "test", client: &client}).GetResource(context.Background())
	require.NoError(t, err)

	var ipv6 []string
	for _, r := range resources.Items {
		if r.PublicIPv6 != "" {
			ipv6 = append(ipv6, r.PublicIPv6)
		}
	}
	assert.Equal(t, []string{"2600:3c03::f03c:91ff:fe24:3a2f"}, ipv6)
}
