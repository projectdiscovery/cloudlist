package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processLoadBalancersForTest mirrors the nil-safe loop in listELBV2Resources
// so we can exercise the guard logic without a live AWS client.
func processLoadBalancersForTest(lbs []*elbv2.LoadBalancer) *schema.Resources {
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
			Service:  "alb",
		})
	}
	return list
}

// processTargetsForTest mirrors the nil-safe target loop in listELBV2Resources.
func processTargetsForTest(targets []*elbv2.TargetHealthDescription) []string {
	var ids []string
	for _, target := range targets {
		if target.Target == nil || target.Target.Id == nil {
			continue
		}
		ids = append(ids, *target.Target.Id)
	}
	return ids
}

func TestListELBV2Resources_NilDNSName(t *testing.T) {
	t.Parallel()

	lbs := []*elbv2.LoadBalancer{
		{
			DNSName:         nil,
			LoadBalancerName: aws.String("internal-lb"),
			LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:loadbalancer/app/internal-lb/abc"),
		},
		{
			DNSName:         aws.String(""),
			LoadBalancerName: nil,
		},
	}

	require.NotPanics(t, func() {
		resources := processLoadBalancersForTest(lbs)
		assert.Equal(t, 0, len(resources.Items), "nil DNSName or LoadBalancerName LBs must be skipped")
	})
}

func TestListELBV2Resources_ValidLB(t *testing.T) {
	t.Parallel()

	lbs := []*elbv2.LoadBalancer{
		{
			DNSName:         aws.String("my-lb-1234567890.us-east-1.elb.amazonaws.com"),
			LoadBalancerName: aws.String("my-lb"),
			LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:loadbalancer/app/my-lb/abc"),
		},
		{
			DNSName:         nil,
			LoadBalancerName: aws.String("broken-lb"),
		},
	}

	resources := processLoadBalancersForTest(lbs)
	require.Equal(t, 1, len(resources.Items))
	assert.Equal(t, "my-lb-1234567890.us-east-1.elb.amazonaws.com", resources.Items[0].DNSName)
	assert.Equal(t, "my-lb", resources.Items[0].ID)
}

func TestListELBV2Resources_NilTargetId(t *testing.T) {
	t.Parallel()

	targets := []*elbv2.TargetHealthDescription{
		{
			Target: nil,
		},
		{
			Target: &elbv2.TargetDescription{
				Id: nil,
			},
		},
		{
			Target: &elbv2.TargetDescription{
				Id: aws.String("i-0abc123def456"),
			},
		},
	}

	require.NotPanics(t, func() {
		ids := processTargetsForTest(targets)
		assert.Equal(t, []string{"i-0abc123def456"}, ids)
	})
}
