package aws

import (
	"context"
	"fmt"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

func TestWorkerPanicRecovery(t *testing.T) {
	results := make(chan result, 1)

	worker(context.Background(), func(ctx context.Context) (*schema.Resources, error) {
		panic("simulated nil pointer in provider")
	}, results)

	res := <-results
	if res.err == nil {
		t.Fatal("expected error from panicking worker, got nil")
	}
	if res.resources != nil {
		t.Fatal("expected nil resources from panicking worker")
	}
	t.Logf("worker panic recovered: %v", res.err)
}

func TestWorkerNormalOperation(t *testing.T) {
	results := make(chan result, 1)
	expected := schema.NewResources()
	expected.Append(&schema.Resource{Provider: "test", DNSName: "example.com"})

	worker(context.Background(), func(ctx context.Context) (*schema.Resources, error) {
		return expected, nil
	}, results)

	res := <-results
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if len(res.resources.Items) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(res.resources.Items))
	}
}

func TestWorkerErrorPropagation(t *testing.T) {
	results := make(chan result, 1)

	worker(context.Background(), func(ctx context.Context) (*schema.Resources, error) {
		return nil, fmt.Errorf("aws api error")
	}, results)

	res := <-results
	if res.err == nil || res.err.Error() != "aws api error" {
		t.Fatalf("expected 'aws api error', got: %v", res.err)
	}
}
