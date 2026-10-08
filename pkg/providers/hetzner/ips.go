package hetzner

import (
	"context"
	"net"

	hetzner "github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// floatingIPProvider is a floating IP provider for Hetzner Cloud API
type floatingIPProvider struct {
	id     string
	client *hetzner.Client
}

func (p *floatingIPProvider) name() string {
	return "floatingip"
}

// GetResource returns all the floating IPs, assigned or not.
func (p *floatingIPProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	floatingIPs, err := p.client.FloatingIP.All(ctx)
	if err != nil {
		return nil, err
	}

	for _, ip := range floatingIPs {
		list.Append(publicIPResource(p.id, p.name(), ip.IP))
	}
	return list, nil
}

// primaryIPProvider is a primary IP provider for Hetzner Cloud API
type primaryIPProvider struct {
	id     string
	client *hetzner.Client
}

func (p *primaryIPProvider) name() string {
	return "primaryip"
}

// GetResource returns all the primary IPs, including unassigned ones that
// the server listing cannot surface.
func (p *primaryIPProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	primaryIPs, err := p.client.PrimaryIP.All(ctx)
	if err != nil {
		return nil, err
	}

	for _, ip := range primaryIPs {
		list.Append(publicIPResource(p.id, p.name(), ip.IP))
	}
	return list, nil
}

func publicIPResource(id, service string, ip net.IP) *schema.Resource {
	resource := &schema.Resource{
		Provider: providerName,
		ID:       id,
		Public:   true,
		Service:  service,
	}
	if ip.To4() != nil {
		resource.PublicIPv4 = ip.String()
	} else {
		resource.PublicIPv6 = ipString(ip)
	}
	return resource
}

// ipString avoids net.IP's "<nil>" rendering for unset addresses.
func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}
