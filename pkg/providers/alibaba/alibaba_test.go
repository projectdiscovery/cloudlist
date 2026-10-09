package alibaba

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAlibabaAPI answers the RPC (Action=...) and ROA (path) calls used by the
// provider. SLB and ALB return two pages to exercise pagination.
func fakeAlibabaAPI(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")

		var body string
		switch {
		case r.Form.Get("Action") == "DescribeLoadBalancers" && r.Form.Get("PageNumber") == "1":
			body = `{"TotalCount":101,"LoadBalancers":{"LoadBalancer":[{"Address":"47.88.1.10","AddressType":"internet"}]}}`
		case r.Form.Get("Action") == "DescribeLoadBalancers" && r.Form.Get("PageNumber") == "2":
			body = `{"TotalCount":101,"LoadBalancers":{"LoadBalancer":[{"Address":"10.0.0.11","AddressType":"intranet"}]}}`
		case r.Form.Get("Action") == "ListLoadBalancers" && r.Form.Get("NextToken") == "":
			body = `{"NextToken":"page-2","LoadBalancers":[{"DNSName":"alb-public.cn-hangzhou.alb.aliyuncs.com","AddressType":"Internet"}]}`
		case r.Form.Get("Action") == "ListLoadBalancers" && r.Form.Get("NextToken") == "page-2":
			body = `{"LoadBalancers":[{"DNSName":"alb-internal.cn-hangzhou.alb.aliyuncs.com","AddressType":"Intranet"}]}`
		case r.Form.Get("Action") == "DescribeEipAddresses":
			body = `{"TotalCount":1,"EipAddresses":{"EipAddress":[{"IpAddress":"47.88.1.30","Status":"Available"}]}}`
		case r.URL.Path == "/api/v1/clusters":
			body = `{"clusters":[` +
				`{"master_url":"{\"api_server_endpoint\":\"https://47.88.1.40:6443\",\"intranet_api_server_endpoint\":\"https://192.168.0.10:6443\"}"},` +
				`{"master_url":""}` +
				`],"page_info":{"total_count":2}}`
		default:
			t.Errorf("unexpected request: %s %s", r.URL.Path, r.Form.Encode())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestResources(t *testing.T) {
	server := fakeAlibabaAPI(t)
	domain := strings.TrimPrefix(server.URL, "http://")
	config := sdk.NewConfig().WithScheme("HTTP")
	credential := credentials.NewAccessKeyCredential("test", "test")

	slbClient, err := slb.NewClientWithOptions("cn-hangzhou", config, credential)
	require.NoError(t, err)
	albClient, err := alb.NewClientWithOptions("cn-hangzhou", config, credential)
	require.NoError(t, err)
	vpcClient, err := vpc.NewClientWithOptions("cn-hangzhou", config, credential)
	require.NoError(t, err)
	csClient, err := cs.NewClientWithOptions("cn-hangzhou", config, credential)
	require.NoError(t, err)
	slbClient.Domain, albClient.Domain, vpcClient.Domain, csClient.Domain = domain, domain, domain, domain

	provider := &Provider{id: "test", slbClient: slbClient, albClient: albClient, vpcClient: vpcClient, csClient: csClient}
	resources, err := provider.Resources(context.Background())
	require.NoError(t, err)

	got := map[string]*schema.Resource{}
	for _, item := range resources.Items {
		for _, value := range []string{item.PublicIPv4, item.PrivateIpv4, item.DNSName} {
			if value != "" {
				got[value] = item
			}
		}
	}

	expected := map[string]struct {
		service string
		public  bool
	}{
		"47.88.1.10": {"slb", true},
		"10.0.0.11":  {"slb", false},
		"alb-public.cn-hangzhou.alb.aliyuncs.com": {"alb", true},
		// schema.Resources marks every DNS name public, whatever the provider set.
		"alb-internal.cn-hangzhou.alb.aliyuncs.com": {"alb", true},
		"47.88.1.30":   {"eip", true},
		"47.88.1.40":   {"ack", true},
		"192.168.0.10": {"ack", false},
	}
	require.Len(t, got, len(expected))
	for value, want := range expected {
		resource, ok := got[value]
		require.True(t, ok, "missing %s", value)
		assert.Equal(t, want.service, resource.Service, value)
		assert.Equal(t, want.public, resource.Public, value)
		assert.Equal(t, providerName, resource.Provider, value)
	}
}

func TestResourcesKeepsEarlierSLBPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("Action") == "DescribeLoadBalancers" && r.Form.Get("PageNumber") == "1" {
			_, _ = w.Write([]byte(`{"TotalCount":101,"LoadBalancers":{"LoadBalancer":[{"Address":"47.88.1.10","AddressType":"internet"}]}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	config := sdk.NewConfig().WithScheme("HTTP").WithAutoRetry(false)
	client, err := slb.NewClientWithOptions("cn-hangzhou", config, credentials.NewAccessKeyCredential("test", "test"))
	require.NoError(t, err)
	client.Domain = strings.TrimPrefix(server.URL, "http://")

	resources, err := (&Provider{id: "test", slbClient: client}).Resources(context.Background())
	require.NoError(t, err)

	var public []string
	for _, item := range resources.Items {
		if item.PublicIPv4 != "" {
			public = append(public, item.PublicIPv4)
		}
	}
	assert.Equal(t, []string{"47.88.1.10"}, public)
}
