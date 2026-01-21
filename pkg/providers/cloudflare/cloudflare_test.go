package cloudflare

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudflare/cloudflare-go"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/require"
)

type failingZonesClient struct {
	err error
}

func (f *failingZonesClient) ListZones(context.Context, ...string) ([]cloudflare.Zone, error) {
	return nil, f.err
}

func (f *failingZonesClient) ListDNSRecords(context.Context, *cloudflare.ResourceContainer, cloudflare.ListDNSRecordsParams) ([]cloudflare.DNSRecord, *cloudflare.ResultInfo, error) {
	return nil, nil, nil
}

type failingRecordsClient struct {
	zones []cloudflare.Zone
	err   error
}

func (f *failingRecordsClient) ListZones(context.Context, ...string) ([]cloudflare.Zone, error) {
	return f.zones, nil
}

func (f *failingRecordsClient) ListDNSRecords(context.Context, *cloudflare.ResourceContainer, cloudflare.ListDNSRecordsParams) ([]cloudflare.DNSRecord, *cloudflare.ResultInfo, error) {
	return nil, nil, f.err
}

func TestProviderResourcesPropagatesZoneError(t *testing.T) {
	t.Parallel()

	p := &Provider{
		id: "test",
		client: &failingZonesClient{
			err: errors.New("zones down"),
		},
		services: schema.ServiceMap{"dns": struct{}{}},
	}

	_, err := p.Resources(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "zones down")
}

func TestProviderResourcesPropagatesRecordError(t *testing.T) {
	t.Parallel()

	p := &Provider{
		id: "test",
		client: &failingRecordsClient{
			zones: []cloudflare.Zone{{ID: "zone-id"}},
			err:   errors.New("record fetch failed"),
		},
		services: schema.ServiceMap{"dns": struct{}{}},
	}

	_, err := p.Resources(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "record fetch failed")
}
