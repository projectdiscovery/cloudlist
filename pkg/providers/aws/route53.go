package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// route53Provider is a provider for aws Route53 API
type route53Provider struct {
	id      string
	route53 *route53.Route53
	session *session.Session
}

// GetResource returns all the resources in the store for a provider.
func (d *route53Provider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	req := &route53.ListHostedZonesInput{}
	for {
		zoneOutput, err := d.route53.ListHostedZones(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not list hosted zones")
		}
		for _, zone := range zoneOutput.HostedZones {
			items, err := d.listResourceRecords(zone)
			if err != nil {
				return nil, errors.Wrap(err, "could not list hosted zones records")
			}
			list.Merge(items)
		}
		if aws.BoolValue(zoneOutput.IsTruncated) && *zoneOutput.NextMarker != "" {
			req.SetMarker(*zoneOutput.Marker)
		} else {
			return list, nil
		}
	}
}

// listResourceRecords lists the resource records for a hosted route53 zone.
func (d *route53Provider) listResourceRecords(zone *route53.HostedZone) (*schema.Resources, error) {
	public := !*zone.Config.PrivateZone
	req := &route53.ListResourceRecordSetsInput{HostedZoneId: aws.String(*zone.Id)}
	list := schema.NewResources()

	for {
		sets, err := d.route53.ListResourceRecordSets(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not list resource_record set")
		}
		for _, item := range sets.ResourceRecordSets {
			if *item.Type != "A" && *item.Type != "CNAME" && *item.Type != "AAAA" {
				continue
			}
			name := strings.TrimSuffix(*item.Name, ".")

			var ip4 string
			if len(item.ResourceRecords) >= 1 {
				ip4 = aws.StringValue(item.ResourceRecords[0].Value)
			}
			list.Append(&schema.Resource{
				ID:       d.id,
				Public:   public,
				DNSName:  name,
				Provider: providerName,
			})
			list.Append(&schema.Resource{
				ID:         d.id,
				Public:     public,
				PublicIPv4: ip4,
				Provider:   providerName,
			})
		}
		if aws.BoolValue(sets.IsTruncated) && *sets.NextRecordName != "" {
			req.SetStartRecordName(*sets.NextRecordName)
		} else {
			return list, nil
		}
	}
}
