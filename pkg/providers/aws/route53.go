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
	"github.com/projectdiscovery/gologger"
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
			defer func() {
				if r := recover(); r != nil {
					gologger.Error().Msgf("panic in %s provider goroutine: %v", "route53", r)
				}
			}()

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

		// Extract zone metadata for this zone
		var zoneMetadata map[string]string
		if r.options.ExtendedMetadata {
			zoneMetadata = r.getRoute53Metadata(zone, client)
		}

		req := &route53.ListResourceRecordSetsInput{HostedZoneId: aws.String(*zone.Id)}
		for {
			sets, err := client.ListResourceRecordSets(req)
			if err != nil {
				return nil, errors.Wrap(err, "could not list resource_record set")
			}
			for _, item := range sets.ResourceRecordSets {
				name := strings.TrimSuffix(*item.Name, ".")

				var record string
				if len(item.ResourceRecords) >= 1 {
					record = aws.StringValue(item.ResourceRecords[0].Value)
				}

				// Create metadata for DNS record
				metadata := make(map[string]string)
				if r.options.ExtendedMetadata {
					for k, v := range zoneMetadata {
						metadata[k] = v
					}
					metadata["record_name"] = name
					schema.AddMetadata(metadata, "record_type", item.Type)
					if item.TTL != nil {
						metadata["ttl"] = fmt.Sprintf("%d", aws.Int64Value(item.TTL))
					}
					schema.AddMetadata(metadata, "set_identifier", item.SetIdentifier)
					if len(item.ResourceRecords) > 0 {
						var records []string
						for _, rr := range item.ResourceRecords {
							if rr.Value != nil {
								records = append(records, aws.StringValue(rr.Value))
							}
						}
						metadata["record_values"] = strings.Join(records, ",")
						schema.AddMetadataInt(metadata, "record_count", len(item.ResourceRecords))
					}
				}

				list.Append(&schema.Resource{
					ID:       r.options.Id,
					Public:   public,
					DNSName:  name,
					Provider: providerName,
					Service:  r.name(),
					Metadata: metadata,
				})

				resource := &schema.Resource{
					ID:       r.options.Id,
					Public:   public,
					Provider: providerName,
					Service:  r.name(),
				}

				//nolint
				if *item.Type == "A" {
					resource.PublicIPv4 = record
				} else if *item.Type == "AAAA" {
					resource.PublicIPv6 = record
				}

				// Add metadata to IP resources too
				if r.options.ExtendedMetadata && (*item.Type == "A" || *item.Type == "AAAA") {
					ipMetadata := make(map[string]string)
					for k, v := range zoneMetadata {
						ipMetadata[k] = v
					}
					ipMetadata["record_name"] = name
					schema.AddMetadata(ipMetadata, "record_type", item.Type)
					if item.TTL != nil {
						ipMetadata["ttl"] = fmt.Sprintf("%d", aws.Int64Value(item.TTL))
					}
					resource.Metadata = ipMetadata
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

func (r *route53Provider) getRoute53Metadata(zone *route53.HostedZone, client *route53.Route53) map[string]string {
	metadata := make(map[string]string)

	// Basic zone information
	if zone.Id != nil {
		zoneId := aws.StringValue(zone.Id)
		metadata["zone_id"] = zoneId
	}
	schema.AddMetadata(metadata, "zone_name", zone.Name)
	if zone.Config != nil && zone.Config.PrivateZone != nil {
		metadata["private_zone"] = fmt.Sprintf("%v", aws.BoolValue(zone.Config.PrivateZone))
	}
	if zone.ResourceRecordSetCount != nil {
		schema.AddMetadataInt(metadata, "record_count", int(aws.Int64Value(zone.ResourceRecordSetCount)))
	}
	if zone.Config != nil {
		schema.AddMetadata(metadata, "comment", zone.Config.Comment)
	}

	schema.AddMetadata(metadata, "caller_reference", zone.CallerReference)

	// Get zone tags
	if zone.Id != nil {
		if tagOutput, err := client.ListTagsForResource(&route53.ListTagsForResourceInput{
			ResourceType: aws.String("hostedzone"),
			ResourceId:   zone.Id,
		}); err == nil && tagOutput.ResourceTagSet != nil && tagOutput.ResourceTagSet.Tags != nil {
			if tagString := buildRoute53TagString(tagOutput.ResourceTagSet.Tags); tagString != "" {
				metadata["tags"] = tagString
			}
		}
	}

	return metadata
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

func buildRoute53TagString(tags []*route53.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
