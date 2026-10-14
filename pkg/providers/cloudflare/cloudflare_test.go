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

type trackingClient struct {
	zones            []cloudflare.Zone
	listZonesCalls   int
	listDNSCalls     int
	lastZoneID       string
	lastListDNSParam cloudflare.ListDNSRecordsParams
}

func (t *trackingClient) ListZones(context.Context, ...string) ([]cloudflare.Zone, error) {
	t.listZonesCalls++
	return t.zones, nil
}

func (t *trackingClient) ListDNSRecords(_ context.Context, zoneID *cloudflare.ResourceContainer, params cloudflare.ListDNSRecordsParams) ([]cloudflare.DNSRecord, *cloudflare.ResultInfo, error) {
	t.listDNSCalls++
	if zoneID != nil {
		t.lastZoneID = zoneID.Identifier
	}
	t.lastListDNSParam = params
	return nil, &cloudflare.ResultInfo{}, nil
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

func TestProviderVerifyPropagatesZoneError(t *testing.T) {
	t.Parallel()

	p := &Provider{
		id: "test",
		client: &failingZonesClient{
			err: errors.New("zones down"),
		},
		services: schema.ServiceMap{"dns": struct{}{}},
	}

	err := p.Verify(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "zones down")
}

func TestProviderVerifyReturnsErrorWhenNoZonesExist(t *testing.T) {
	t.Parallel()

	client := &trackingClient{}
	p := &Provider{
		id:       "test",
		client:   client,
		services: schema.ServiceMap{"dns": struct{}{}},
	}

	err := p.Verify(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no accessible Cloudflare zones found")
	require.Equal(t, 1, client.listZonesCalls)
	require.Zero(t, client.listDNSCalls)
}

func TestProviderVerifySkipsWhenDNSServiceDisabled(t *testing.T) {
	t.Parallel()

	client := &trackingClient{}
	p := &Provider{
		id:       "test",
		client:   client,
		services: schema.ServiceMap{"workers": struct{}{}},
	}

	err := p.Verify(context.Background())
	require.NoError(t, err)
	require.Zero(t, client.listZonesCalls)
	require.Zero(t, client.listDNSCalls)
}

func TestProviderVerifyUsesSingleRecordProbe(t *testing.T) {
	t.Parallel()

	client := &trackingClient{
		zones: []cloudflare.Zone{{ID: "zone-id", Name: "example.com"}},
	}
	p := &Provider{
		id:       "test",
		client:   client,
		services: schema.ServiceMap{"dns": struct{}{}},
	}

	err := p.Verify(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, client.listZonesCalls)
	require.Equal(t, 1, client.listDNSCalls)
	require.Equal(t, "zone-id", client.lastZoneID)
	require.Equal(t, 1, client.lastListDNSParam.Page)
	require.Equal(t, 1, client.lastListDNSParam.PerPage)
}

func TestProviderVerifyPropagatesRecordError(t *testing.T) {
	t.Parallel()

	p := &Provider{
		id: "test",
		client: &failingRecordsClient{
			zones: []cloudflare.Zone{{ID: "zone-id", Name: "example.com"}},
			err:   errors.New("record fetch failed"),
		},
		services: schema.ServiceMap{"dns": struct{}{}},
	}

	err := p.Verify(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to verify Cloudflare DNS access")
	require.Contains(t, err.Error(), "record fetch failed")
}
