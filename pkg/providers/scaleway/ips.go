package scaleway

import (
	"context"
	"fmt"
	"net"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	"github.com/scaleway/scaleway-sdk-go/api/flexibleip/v1alpha1"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/api/lb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ipProvider lists reserved IPs, which are returned whether or not they are
// attached, so detached IPs that ListServers never sees are covered too.
type ipProvider struct {
	id            string
	instanceAPI   *instance.API
	flexibleIPAPI *flexibleip.API
	lbAPI         *lb.ZonedAPI
}

// GetFlexibleIPs returns Instance flexible IPs and Elastic Metal flexible IPs.
func (d *ipProvider) GetFlexibleIPs(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	instanceErr := forEachLocality(d.instanceAPI.Zones(), func(zone scw.Zone) error {
		resp, err := d.instanceAPI.ListIPs(&instance.ListIPsRequest{Zone: zone}, scw.WithAllPages(), scw.WithContext(ctx))
		if err != nil {
			return err
		}
		for _, ip := range resp.IPs {
			list.Append(d.ipResource(ip.Address, "flexibleip"))
		}
		return nil
	})

	metalErr := forEachLocality(d.flexibleIPAPI.Zones(), func(zone scw.Zone) error {
		resp, err := d.flexibleIPAPI.ListFlexibleIPs(&flexibleip.ListFlexibleIPsRequest{Zone: zone}, scw.WithAllPages(), scw.WithContext(ctx))
		if err != nil {
			return err
		}
		for _, ip := range resp.FlexibleIPs {
			list.Append(d.ipResource(ip.IPAddress.IP, "flexibleip"))
		}
		return nil
	})
	if len(list.Items) == 0 && instanceErr != nil && metalErr != nil {
		return nil, fmt.Errorf("scaleway: flexible ip listing failed: %v; %v", instanceErr, metalErr)
	}
	for _, err := range []error{instanceErr, metalErr} {
		if err != nil {
			gologger.Warning().Msgf("scaleway: flexible ip listing failed: %v", err)
		}
	}
	return list, nil
}

// GetLoadBalancerIPs returns load balancer IPs.
func (d *ipProvider) GetLoadBalancerIPs(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	err := forEachLocality(d.lbAPI.Zones(), func(zone scw.Zone) error {
		resp, err := d.lbAPI.ListIPs(&lb.ZonedAPIListIPsRequest{Zone: zone}, scw.WithAllPages(), scw.WithContext(ctx))
		if err != nil {
			return err
		}
		for _, ip := range resp.IPs {
			list.Append(d.ipResource(net.ParseIP(ip.IPAddress), "lb"))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (d *ipProvider) ipResource(ip net.IP, service string) *schema.Resource {
	resource := &schema.Resource{
		Provider: providerName,
		ID:       d.id,
		Public:   true,
		Service:  service,
	}
	if ip.To4() != nil {
		resource.PublicIPv4 = ip.String()
	} else if ip != nil {
		resource.PublicIPv6 = ip.String()
	}
	return resource
}

// forEachLocality calls fn for every zone or region, skipping ones that fail
// (e.g. a product not offered there), and only errors when all of them failed.
func forEachLocality[T scw.Zone | scw.Region](localities []T, fn func(T) error) error {
	var errs []error
	for _, locality := range localities {
		if err := fn(locality); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 && len(errs) == len(localities) {
		return fmt.Errorf("all localities failed: %v", errs)
	}
	return nil
}
