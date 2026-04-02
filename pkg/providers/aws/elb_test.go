package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processELBLoadBalancersForTest mirrors the nil-safe loop in listELBResources
// so we can exercise the guard logic without a live AWS client.
func processELBLoadBalancersForTest(lbs []*elb.LoadBalancerDescription) *schema.Resources {
	list := schema.NewResources()
	for _, lb := range lbs {
		if lb.DNSName == nil || lb.LoadBalancerName == nil {
			continue
		}
		list.Append(&schema.Resource{
			Provider: "aws",
			ID:       *lb.LoadBalancerName,
			DNSName:  *lb.DNSName,
			Public:   true,
			Service:  "elb",
		})
	}
	return list
}

// processELBInstancesForTest mirrors the nil-safe instance loop in listELBResources.
func processELBInstancesForTest(instances []*elb.Instance) []string {
	var ids []string
	for _, instance := range instances {
		if instance.InstanceId == nil {
			continue
		}
		ids = append(ids, *instance.InstanceId)
	}
	return ids
}

func TestListELBResources_NilDNSName(t *testing.T) {
	t.Parallel()

	lbs := []*elb.LoadBalancerDescription{
		{
			DNSName:          nil,
			LoadBalancerName: aws.String("internal-classic-lb"),
		},
		{
			DNSName:          aws.String(""),
			LoadBalancerName: nil,
		},
	}

	require.NotPanics(t, func() {
		resources := processELBLoadBalancersForTest(lbs)
		assert.Equal(t, 0, len(resources.Items), "nil DNSName or LoadBalancerName LBs must be skipped")
	})
}

func TestListELBResources_ValidLB(t *testing.T) {
	t.Parallel()

	lbs := []*elb.LoadBalancerDescription{
		{
			DNSName:          aws.String("my-classic-lb-1234567890.us-east-1.elb.amazonaws.com"),
			LoadBalancerName: aws.String("my-classic-lb"),
		},
		{
			DNSName:          nil,
			LoadBalancerName: aws.String("broken-lb"),
		},
	}

	resources := processELBLoadBalancersForTest(lbs)
	require.Equal(t, 1, len(resources.Items))
	assert.Equal(t, "my-classic-lb-1234567890.us-east-1.elb.amazonaws.com", resources.Items[0].DNSName)
	assert.Equal(t, "my-classic-lb", resources.Items[0].ID)
}

func TestListELBResources_NilInstanceId(t *testing.T) {
	t.Parallel()

	instances := []*elb.Instance{
		{InstanceId: nil},
		{InstanceId: aws.String("i-0abc123def456")},
		{InstanceId: aws.String("i-0def456abc123")},
	}

	require.NotPanics(t, func() {
		ids := processELBInstancesForTest(instances)
		assert.Equal(t, []string{"i-0abc123def456", "i-0def456abc123"}, ids)
	})
}
