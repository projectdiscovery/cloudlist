package aws

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const describeAddressesResponse = `<?xml version="1.0" encoding="UTF-8"?>
<DescribeAddressesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>f7de5e98-491a-4c19-a92d-908d6EXAMPLE</requestId>
  <addressesSet>
    <item>
      <publicIp>54.239.28.10</publicIp>
      <allocationId>eipalloc-attached</allocationId>
      <domain>vpc</domain>
      <instanceId>i-0123456789abcdef0</instanceId>
      <associationId>eipassoc-1</associationId>
      <privateIpAddress>10.0.0.5</privateIpAddress>
    </item>
    <item>
      <publicIp>54.239.28.20</publicIp>
      <allocationId>eipalloc-unattached</allocationId>
      <domain>vpc</domain>
      <tagSet>
        <item><key>Name</key><value>reserved</value></item>
      </tagSet>
    </item>
    <item>
      <allocationId>eipalloc-no-ip</allocationId>
      <domain>vpc</domain>
    </item>
  </addressesSet>
</DescribeAddressesResponse>`

func newTestEC2Client(t *testing.T, body string) *ec2.EC2 {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "DescribeAddresses", r.Form.Get("Action"))
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(server.URL),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
	})
	require.NoError(t, err)
	return ec2.New(sess)
}

func TestGetElasticIPResources(t *testing.T) {
	provider := &instanceProvider{options: ProviderOptions{Id: "test", ExtendedMetadata: true}}

	resources, err := provider.getElasticIPResources(newTestEC2Client(t, describeAddressesResponse))
	require.NoError(t, err)
	require.Len(t, resources.Items, 2, "addresses without a public IP must be skipped")

	attached, unattached := resources.Items[0], resources.Items[1]

	assert.Equal(t, "54.239.28.10", attached.PublicIPv4)
	assert.Equal(t, "eip", attached.Service)
	assert.True(t, attached.Public)
	assert.Equal(t, "i-0123456789abcdef0", attached.Metadata["instance_id"])

	assert.Equal(t, "54.239.28.20", unattached.PublicIPv4)
	assert.Equal(t, "eipalloc-unattached", unattached.Metadata["allocation_id"])
	assert.Equal(t, "Name=reserved", unattached.Metadata["tags"])
	assert.NotContains(t, unattached.Metadata, "instance_id")
}
