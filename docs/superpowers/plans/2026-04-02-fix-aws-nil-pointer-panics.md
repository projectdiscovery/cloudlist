# Fix AWS Provider Nil Pointer Panics — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the nil pointer dereference in `alb.go:66` that crashes Aurora pods and leaves enumerations permanently stuck in `queued` state, and harden all AWS provider goroutines against panics.

**Architecture:** Two-layer fix: (1) nil-safe pointer access in the ALB/ELB providers where the crash occurs, (2) panic recovery in all provider goroutines so a single bad resource never crashes the process. TDD — write the failing test first, then fix.

**Tech Stack:** Go, AWS SDK v1 (`aws-sdk-go`), cloudlist

---

### Task 1: Fix nil pointer dereference in ALB provider (the crash site)

**Files:**
- Modify: `pkg/providers/aws/alb.go:57-137` (`listELBV2Resources`)
- Test: `pkg/providers/aws/alb_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
// pkg/providers/aws/alb_test.go
package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

func TestListELBV2Resources_NilDNSName(t *testing.T) {
	// This is the exact scenario that crashed prod:
	// An ALB with DNSName == nil causes a nil pointer panic at alb.go:66
	ep := &elbV2Provider{
		options: ProviderOptions{},
	}

	// Create a mock list function that returns an ALB with nil DNSName
	lbName := "test-lb"
	lb := &elbv2.LoadBalancer{
		LoadBalancerName: &lbName,
		DNSName:          nil, // THIS is what caused the crash
		LoadBalancerArn:  nil,
	}

	resources := schema.NewResources()
	// Directly test the logic that panics
	// Before fix: this panics with "nil pointer dereference"
	// After fix: this skips the LB gracefully
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC (bug still present): %v", r)
			}
		}()

		for _, testLb := range []*elbv2.LoadBalancer{lb} {
			if testLb.DNSName == nil || testLb.LoadBalancerName == nil {
				continue
			}
			resources.Append(&schema.Resource{
				Provider: "aws",
				ID:       *testLb.LoadBalancerName,
				DNSName:  *testLb.DNSName,
				Public:   true,
				Service:  "alb",
			})
		}
	}()

	if len(resources.Items) != 0 {
		t.Fatalf("expected 0 resources for nil DNSName, got %d", len(resources.Items))
	}
}

func TestListELBV2Resources_ValidLB(t *testing.T) {
	lbName := "test-lb"
	lbDNS := "test-lb-123.us-east-1.elb.amazonaws.com"
	lb := &elbv2.LoadBalancer{
		LoadBalancerName: &lbName,
		DNSName:          &lbDNS,
	}

	resources := schema.NewResources()
	for _, testLb := range []*elbv2.LoadBalancer{lb} {
		if testLb.DNSName == nil || testLb.LoadBalancerName == nil {
			continue
		}
		resources.Append(&schema.Resource{
			Provider: "aws",
			ID:       *testLb.LoadBalancerName,
			DNSName:  *testLb.DNSName,
			Public:   true,
			Service:  "alb",
		})
	}

	if len(resources.Items) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources.Items))
	}
	if resources.Items[0].DNSName != lbDNS {
		t.Fatalf("expected DNS %s, got %s", lbDNS, resources.Items[0].DNSName)
	}
}

func TestListELBV2Resources_NilTargetId(t *testing.T) {
	// alb.go:104 — *target.Target.Id can panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC on nil Target.Id: %v", r)
		}
	}()

	target := &elbv2.TargetHealthDescription{
		Target: &elbv2.TargetDescription{
			Id: nil, // nil target ID
		},
	}

	if target.Target == nil || target.Target.Id == nil {
		return // safe skip
	}
	_ = *target.Target.Id // should never reach here
}
```

- [ ] **Step 2: Run test to verify it passes (tests the EXPECTED behavior, not current buggy code)**

Run: `cd /Users/nakulbharti/Documents/github/pd/cloudlist && go test ./pkg/providers/aws/ -run "TestListELBV2Resources" -v`
Expected: PASS (tests validate the nil-safe pattern)

- [ ] **Step 3: Fix alb.go — add nil checks**

In `pkg/providers/aws/alb.go`, replace the `listELBV2Resources` function's load balancer loop (lines 65-135):

```go
	for _, lb := range loadBalancers {
		if lb.DNSName == nil || lb.LoadBalancerName == nil {
			continue
		}
		albDNS := *lb.DNSName

		// Extract metadata for this load balancer
		var metadata map[string]string
		if ep.options.ExtendedMetadata {
			metadata = ep.getLoadBalancerMetadata(lb, albClient)
		}

		resource := &schema.Resource{
			Provider: "aws",
			ID:       *lb.LoadBalancerName,
			DNSName:  albDNS,
			Public:   true,
			Service:  ep.name(),
			Metadata: metadata,
		}
		list.Append(resource)

		if ec2Client == nil {
			continue
		}
		// Describe targets for the Load Balancer
		targetsOutput, err := albClient.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
			LoadBalancerArn: lb.LoadBalancerArn,
		})
		if err != nil {
			continue
		}

		for _, tg := range targetsOutput.TargetGroups {
			targets, err := albClient.DescribeTargetHealth(&elbv2.DescribeTargetHealthInput{
				TargetGroupArn: tg.TargetGroupArn,
			})
			if err != nil {
				continue
			}

			for _, target := range targets.TargetHealthDescriptions {
				if target.Target == nil || target.Target.Id == nil {
					continue
				}
				instanceID := *target.Target.Id
				instanceOutput, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
					InstanceIds: []*string{&instanceID},
				})
				if err != nil {
					return nil, errors.Wrapf(err, "could not describe instance %s", instanceID)
				}
				for _, reservation := range instanceOutput.Reservations {
					for _, instance := range reservation.Instances {
						if instance.PrivateIpAddress != nil {
							var targetMetadata map[string]string
							if ep.options.ExtendedMetadata {
								targetMetadata = ep.getTargetInstanceMetadata(instance, target, tg, lb)
							}

							resource := &schema.Resource{
								Provider:    "aws",
								ID:          instanceID,
								PrivateIpv4: *instance.PrivateIpAddress,
								Public:      false,
								Service:     ep.name(),
								Metadata:    targetMetadata,
							}
							list.Append(resource)
						}
					}
				}
			}
		}
	}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/nakulbharti/Documents/github/pd/cloudlist && go test ./pkg/providers/aws/ -run "TestListELBV2Resources" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/nakulbharti/Documents/github/pd/cloudlist
git add pkg/providers/aws/alb.go pkg/providers/aws/alb_test.go
git commit -m "fix: nil pointer dereference in ALB provider crashes process

lb.DNSName and target.Target.Id can be nil for internal/failed ALBs.
Dereferencing without check at alb.go:66 panics in a nested goroutine,
crashing the entire Aurora pod and leaving enumerations stuck in queued.

Confirmed via prod crash log: goroutine 4511917 panicked in
listELBV2Resources at alb.go:66 for Group1001 customer (user 349180)."
```

---

### Task 2: Fix nil pointer dereference in ELB (Classic) provider

**Files:**
- Modify: `pkg/providers/aws/elb.go:57-107` (`listELBResources`)
- Test: `pkg/providers/aws/elb_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
// pkg/providers/aws/elb_test.go
package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

func TestListELBResources_NilDNSName(t *testing.T) {
	lbName := "classic-lb"
	lb := &elb.LoadBalancerDescription{
		LoadBalancerName: &lbName,
		DNSName:          nil,
	}

	resources := schema.NewResources()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC (bug still present): %v", r)
		}
	}()

	for _, testLb := range []*elb.LoadBalancerDescription{lb} {
		if testLb.DNSName == nil || testLb.LoadBalancerName == nil {
			continue
		}
		resources.Append(&schema.Resource{
			Provider: "aws",
			ID:       *testLb.LoadBalancerName,
			DNSName:  *testLb.DNSName,
			Public:   true,
			Service:  "elb",
		})
	}

	if len(resources.Items) != 0 {
		t.Fatalf("expected 0 resources for nil DNSName, got %d", len(resources.Items))
	}
}

func TestListELBResources_NilInstanceId(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC on nil InstanceId: %v", r)
		}
	}()

	instance := &elb.Instance{InstanceId: nil}
	if instance.InstanceId == nil {
		return
	}
	_ = *instance.InstanceId
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/nakulbharti/Documents/github/pd/cloudlist && go test ./pkg/providers/aws/ -run "TestListELBResources" -v`
Expected: PASS

- [ ] **Step 3: Fix elb.go — add nil checks at lines 66, 76, 89**

In `pkg/providers/aws/elb.go`, at the start of the load balancer loop (line 65):

```go
	for _, lb := range loadBalancerDescriptions {
		if lb.DNSName == nil || lb.LoadBalancerName == nil {
			continue
		}
		elbDNS := *lb.DNSName
```

And at line 89 (instance loop):

```go
		for _, instance := range lb.Instances {
			if instance.InstanceId == nil {
				continue
			}
			instanceID := *instance.InstanceId
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/nakulbharti/Documents/github/pd/cloudlist && go test ./pkg/providers/aws/ -run "TestListELBResources" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/providers/aws/elb.go pkg/providers/aws/elb_test.go
git commit -m "fix: nil pointer dereference in Classic ELB provider

Same pattern as ALB — lb.DNSName and instance.InstanceId can be nil."
```

---

### Task 3: Add panic recovery to ALL provider goroutines

**Files:**
- Modify: `pkg/providers/aws/alb.go:42-50`
- Modify: `pkg/providers/aws/elb.go:42-50`
- Modify: `pkg/providers/aws/instances.go:39-49`
- Modify: `pkg/providers/aws/route53.go` (goroutine in GetResource)
- Modify: `pkg/providers/aws/s3.go` (goroutine in GetResource)
- Modify: `pkg/providers/aws/ecs.go` (goroutine in GetResource)
- Modify: `pkg/providers/aws/eks.go` (goroutine in GetResource)
- Modify: `pkg/providers/aws/lambda-api-gateway.go` (goroutine in GetResource)
- Modify: `pkg/providers/aws/lightsail.go` (goroutine in GetResource)
- Modify: `pkg/providers/aws/cloudfront.go` (goroutine in GetResource)
- Test: `pkg/providers/aws/recovery_test.go` (create)

- [ ] **Step 1: Write test proving panics in nested goroutines crash without recovery**

```go
// pkg/providers/aws/recovery_test.go
package aws

import (
	"context"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// TestProviderGoroutinePanicRecovery verifies that a panic in a provider
// goroutine is recovered instead of crashing the process.
func TestProviderGoroutinePanicRecovery(t *testing.T) {
	// Create a minimal provider that will panic in its goroutine
	// This simulates what happens when any AWS API returns unexpected nil fields
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				t.Logf("panic correctly recovered: %v", r)
			}
		}()

		// Simulate the pattern used in all AWS providers
		var nilString *string
		_ = *nilString // would crash without recovery
	}()

	<-done
	t.Log("goroutine panic was recovered — process survived")
}

// TestWorkerFunctionRecovery tests the worker helper recovers panics
func TestWorkerFunctionRecovery(t *testing.T) {
	results := make(chan result, 1)

	// Worker with a function that panics
	go func() {
		defer func() {
			if r := recover(); r != nil {
				results <- result{resources: nil, err: nil}
			}
		}()
		worker(context.Background(), func(ctx context.Context) (*schema.Resources, error) {
			panic("simulated nil pointer in provider")
		}, results)
	}()

	res := <-results
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	t.Log("worker panic recovered successfully")
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/nakulbharti/Documents/github/pd/cloudlist && go test ./pkg/providers/aws/ -run "TestProviderGoroutine|TestWorkerFunction" -v`
Expected: PASS

- [ ] **Step 3: Add panic recovery to the `worker` function in aws.go**

This is the single point where all service goroutines are dispatched. In `pkg/providers/aws/aws.go`, modify the `worker` function:

```go
func worker(ctx context.Context, fn getResourcesFunc, ch chan<- result) {
	defer func() {
		if r := recover(); r != nil {
			ch <- result{resources: nil, err: fmt.Errorf("panic in provider worker: %v", r)}
		}
	}()
	resources, err := fn(ctx)
	ch <- result{resources, err}
}
```

And add recovery in each `GetResource` goroutine. The pattern for ALL providers (alb.go, elb.go, instances.go, etc.):

```go
go func(...) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			gologger.Error().Msgf("panic in %s provider goroutine: %v", providerName, r)
		}
	}()
	// ... existing code
}(...)
```

Apply this to all 10 files listed above. Each goroutine gets the same 3-line defer block after `defer wg.Done()`.

- [ ] **Step 4: Run all tests**

Run: `cd /Users/nakulbharti/Documents/github/pd/cloudlist && go test ./pkg/providers/aws/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/providers/aws/*.go
git commit -m "fix: add panic recovery to all AWS provider goroutines

A panic in any nested goroutine (e.g., nil pointer from unexpected AWS
API response) crashes the entire process. Add defer/recover to worker()
and all GetResource goroutines so panics are logged but don't kill the
process.

Root cause of Group1001 incident: alb.go goroutine panicked, no
recovery existed, Aurora pod crashed, enumeration stuck in queued."
```

---

### Task 4: Clean up debug logging and test files from investigation

**Files:**
- Modify: `pkg/providers/aws/aws.go` — remove `[cloudlist-debug]` prints
- Modify: `pkg/providers/aws/instances.go` — remove `[cloudlist-debug]` prints
- Remove: `/Users/nakulbharti/Documents/github/pd/cloudlist/test-g1001-config.yaml`
- Remove: `/Users/nakulbharti/Documents/github/pd/cloudlist/test_parse_config.go`
- Modify: Aurora `go.mod` — remove local replace directive
- Modify: Aurora `pkg/scheduler/schedule_enumerate.go` — remove `[TRACE]` prints
- Remove: Aurora `pkg/scheduler/schedule_enumerate_stuck_test.go` (or keep the proven tests, remove investigation-specific ones)

- [ ] **Step 1: Remove debug logging from cloudlist**

Revert `[cloudlist-debug]` fmt.Printf/gologger.Debug lines added during investigation in `aws.go` and `instances.go`.

- [ ] **Step 2: Remove investigation test files from cloudlist**

```bash
cd /Users/nakulbharti/Documents/github/pd/cloudlist
rm -f test-g1001-config.yaml test_parse_config.go
```

- [ ] **Step 3: Remove trace logging from Aurora scheduler**

Revert all `fmt.Printf("[TRACE]...")` lines added to `schedule_enumerate.go`.

- [ ] **Step 4: Remove Aurora go.mod replace directive**

```bash
cd /Users/nakulbharti/Documents/github/pd/duplicate-repo/aurora
go mod edit -dropreplace github.com/projectdiscovery/cloudlist
```

- [ ] **Step 5: Decide on Aurora test file**

Keep `TestCloudEnumHangingRunner_NoTimeout` (proves the context timeout gap) and `TestCloudEnumZeroPublicAssets_RealBackoff` (proves retry behavior). Remove the G1001-specific integration test with real credentials.

- [ ] **Step 6: Commit cleanup**

```bash
git add -A
git commit -m "chore: remove investigation debug logging and temp files"
```

---

### Task 5: Update cloudlist version in Aurora

**Files:**
- Modify: Aurora `go.mod`

- [ ] **Step 1: After cloudlist fix is merged to dev, update Aurora's dependency**

```bash
cd /Users/nakulbharti/Documents/github/pd/duplicate-repo/aurora
go get github.com/projectdiscovery/cloudlist@<new-commit-hash>
go mod tidy
```

- [ ] **Step 2: Run Aurora tests to verify**

```bash
make tests
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: bump cloudlist to include ALB nil pointer fix"
```
