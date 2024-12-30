package aws

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// route53Provider is a provider for aws Route53 API
type route53Provider struct {
	options ProviderOptions
	route53 *route53.Route53
	session *session.Session
}

func (r *route53Provider) name() string {
	return "route53"
}

// GetResource returns all the resources in the store for a provider.
func (r *route53Provider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, route53Client := range r.getRoute53Clients() {
		wg.Add(1)

		go func(client *route53.Route53) {
			defer wg.Done()

			zones, err := r.getHostedZones(client)
			if err != nil {
				return
			}
			if resources, err := r.listResourcesByZone(zones, client); err == nil {
				mu.Lock()
				list.Merge(resources)
				mu.Unlock()
			}
		}(route53Client)
	}
	wg.Wait()
	return list, nil
}

func (r *route53Provider) getHostedZones(client *route53.Route53) ([]*route53.HostedZone, error) {
	zones := make([]*route53.HostedZone, 0)
	req := &route53.ListHostedZonesInput{}
	for {
		zoneOutput, err := client.ListHostedZones(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not list hosted zones")
		}
		zones = append(zones, zoneOutput.HostedZones...)
		if aws.BoolValue(zoneOutput.IsTruncated) && *zoneOutput.NextMarker != "" {
			req.SetMarker(*zoneOutput.NextMarker)
		} else {
			break
		}
	}
	return zones, nil
}

// listResourceRecords lists the resource records for a hosted route53 zone.
func (r *route53Provider) listResourcesByZone(zones []*route53.HostedZone, client *route53.Route53) (*schema.Resources, error) {
	list := schema.NewResources()
	for _, zone := range zones {
		public := !*zone.Config.PrivateZone

		req := &route53.ListResourceRecordSetsInput{HostedZoneId: aws.String(*zone.Id)}
		for {
			sets, err := client.ListResourceRecordSets(req)
			if err != nil {
				return nil, errors.Wrap(err, "could not list resource_record set")
			}
			for _, item := range sets.ResourceRecordSets {
				if *item.Type != "A" && *item.Type != "CNAME" && *item.Type != "AAAA" {
					continue
				}
				name := strings.TrimSuffix(*item.Name, ".")

				var record string
				if len(item.ResourceRecords) >= 1 {
					record = aws.StringValue(item.ResourceRecords[0].Value)
				}
				list.Append(&schema.Resource{
					ID:       r.options.Id,
					Public:   public,
					DNSName:  name,
					Provider: providerName,
					Service:  r.name(),
				})

				resource := &schema.Resource{
					ID:       r.options.Id,
					Public:   public,
					Provider: providerName,
					Service:  r.name(),
				}

				if *item.Type == "A" {
					resource.PublicIPv4 = record
				} else if *item.Type == "AAAA" {
					resource.PublicIPv6 = record
				}

				list.Append(resource)
			}
			if aws.BoolValue(sets.IsTruncated) && *sets.NextRecordName != "" {
				req.SetStartRecordName(*sets.NextRecordName)
			} else {
				break
			}
		}
	}
	return list, nil
}

func (r *route53Provider) getRoute53Clients() []*route53.Route53 {
	route53Clients := make([]*route53.Route53, 0)
	route53Clients = append(route53Clients, r.route53)

	if r.options.AssumeRoleName == "" || len(r.options.AccountIds) < 1 {
		return route53Clients
	}

	for _, accountId := range r.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, r.options.AssumeRoleName)
		creds := stscreds.NewCredentials(r.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      aws.String("us-east-1"),
			Credentials: creds,
		})
		if err != nil {
			continue
		}

		route53Clients = append(route53Clients, route53.New(assumeSession))
	}
	return route53Clients
}
