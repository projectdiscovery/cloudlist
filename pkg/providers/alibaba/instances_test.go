package alibaba

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstancesAllPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("NextToken") == "page-2" {
			_, _ = w.Write([]byte(`{"RequestId":"2","Instances":{"Instance":[{"PublicIpAddress":{"IpAddress":["47.88.1.2"]}}]},"NextToken":""}`))
			return
		}
		_, _ = w.Write([]byte(`{"RequestId":"1","Instances":{"Instance":[{"PublicIpAddress":{"IpAddress":["47.88.1.1"]}}]},"NextToken":"page-2"}`))
	}))
	t.Cleanup(server.Close)

	client, err := ecs.NewClientWithOptions("cn-hangzhou", sdk.NewConfig().WithScheme("HTTP"), credentials.NewAccessKeyCredential("test", "test"))
	require.NoError(t, err)
	client.Domain = strings.TrimPrefix(server.URL, "http://")

	resources, err := (&instanceProvider{id: "test", client: client}).GetResource(context.Background())
	require.NoError(t, err)

	var ips []string
	for _, r := range resources.Items {
		ips = append(ips, r.PublicIPv4)
	}
	assert.ElementsMatch(t, []string{"47.88.1.1", "47.88.1.2"}, ips)
}
