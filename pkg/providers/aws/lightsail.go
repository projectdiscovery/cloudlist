package aws

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// lightsailProvider is an instance provider for AWS Lightsail API
type lightsailProvider struct {
	options  ProviderOptions
	lsClient *lightsail.Lightsail
	session  *session.Session
	regions  []*lightsail.Region
}

func (l *lightsailProvider) name() string {
	return "lightsail"
}

// GetResource returns all the resources in the store for a provider.
func (l *lightsailProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range l.regions {
		for _, lsClient := range l.getLightsailClients(region.Name) {
			wg.Add(1)

			go func(client *lightsail.Lightsail) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						gologger.Error().Msgf("panic in %s provider goroutine: %v", "lightsail", r)
					}
				}()

				if resources, err := l.listListsailResources(client); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(lsClient)
		}
	}
	wg.Wait()
	return list, nil
}

func (l *lightsailProvider) listListsailResources(lsClient *lightsail.Lightsail) (*schema.Resources, error) {
	list := schema.NewResources()
	req := &lightsail.GetInstancesInput{}
	for {
		resp, err := lsClient.GetInstances(req)
		if err != nil {
			return nil, err
		}

		for _, instance := range resp.Instances {
			privateIPv4 := aws.StringValue(instance.PrivateIpAddress)
			publicIPv4 := aws.StringValue(instance.PublicIpAddress)

			// Extract metadata for this instance
			var metadata map[string]string
			if l.options.ExtendedMetadata {
				metadata = l.getInstanceMetadata(instance)
			}

			resource := &schema.Resource{
				ID:          l.options.Id,
				Provider:    providerName,
				PrivateIpv4: privateIPv4,
				PublicIPv4:  publicIPv4,
				Public:      publicIPv4 != "",
				Service:     l.name(),
				Metadata:    metadata,
			}

			if len(instance.Ipv6Addresses) > 0 {
				resource.PublicIPv6 = aws.StringValue(instance.Ipv6Addresses[0])
			}

			list.Append(resource)
		}
		if aws.StringValue(resp.NextPageToken) == "" {
			break
		}
		req.PageToken = resp.NextPageToken
	}
	return list, nil
}

func (l *lightsailProvider) getInstanceMetadata(instance *lightsail.Instance) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "instance_name", instance.Name)
	schema.AddMetadata(metadata, "blueprint_id", instance.BlueprintId)
	schema.AddMetadata(metadata, "blueprint_name", instance.BlueprintName)
	schema.AddMetadata(metadata, "bundle_id", instance.BundleId)
	schema.AddMetadata(metadata, "ssh_key_name", instance.SshKeyName)
	schema.AddMetadata(metadata, "username", instance.Username)
	schema.AddMetadata(metadata, "resource_type", instance.ResourceType)
	schema.AddMetadata(metadata, "support_code", instance.SupportCode)

	if instance.Arn != nil {
		arn := aws.StringValue(instance.Arn)
		metadata["arn"] = arn

		// Extract owner ID from ARN (format: arn:aws:lightsail:region:account-id:Instance/instance-id)
		if arnComponents := parseARN(arn); arnComponents != nil && arnComponents.AccountID != "" {
			metadata["owner_id"] = arnComponents.AccountID
		}
	}

	if instance.State != nil {
		schema.AddMetadata(metadata, "instance_state", instance.State.Name)
	}
	if instance.IsStaticIp != nil {
		metadata["static_ip"] = fmt.Sprintf("%v", aws.BoolValue(instance.IsStaticIp))
	}
	if instance.Hardware != nil {
		if instance.Hardware.CpuCount != nil {
			schema.AddMetadataInt(metadata, "cpu_count", int(aws.Int64Value(instance.Hardware.CpuCount)))
		}
		if instance.Hardware.RamSizeInGb != nil {
			metadata["ram_size_gb"] = fmt.Sprintf("%.1f", aws.Float64Value(instance.Hardware.RamSizeInGb))
		}
		if len(instance.Hardware.Disks) > 0 {
			var diskSizes []string
			for _, disk := range instance.Hardware.Disks {
				if disk.SizeInGb != nil {
					diskSizes = append(diskSizes, fmt.Sprintf("%dGB", aws.Int64Value(disk.SizeInGb)))
				}
			}
			if len(diskSizes) > 0 {
				metadata["disk_sizes"] = strings.Join(diskSizes, ",")
			}
		}
	}
	if instance.Location != nil {
		schema.AddMetadata(metadata, "availability_zone", instance.Location.AvailabilityZone)
		schema.AddMetadata(metadata, "region", instance.Location.RegionName)
	}
	if instance.CreatedAt != nil {
		metadata["created_at"] = instance.CreatedAt.Format(time.RFC3339)
	}

	if instance.Networking != nil {
		if len(instance.Networking.Ports) > 0 {
			var openPorts []string
			for _, port := range instance.Networking.Ports {
				if port.FromPort != nil && port.ToPort != nil && port.Protocol != nil {
					if aws.Int64Value(port.FromPort) == aws.Int64Value(port.ToPort) {
						openPorts = append(openPorts, fmt.Sprintf("%d/%s",
							aws.Int64Value(port.FromPort), aws.StringValue(port.Protocol)))
					} else {
						openPorts = append(openPorts, fmt.Sprintf("%d-%d/%s",
							aws.Int64Value(port.FromPort), aws.Int64Value(port.ToPort),
							aws.StringValue(port.Protocol)))
					}
				}
			}
			if len(openPorts) > 0 {
				metadata["open_ports"] = strings.Join(openPorts, ",")
			}
		}

		// Monthly transfer information
		if instance.Networking.MonthlyTransfer != nil && instance.Networking.MonthlyTransfer.GbPerMonthAllocated != nil {
			schema.AddMetadataInt(metadata, "monthly_transfer_gb", int(aws.Int64Value(instance.Networking.MonthlyTransfer.GbPerMonthAllocated)))
		}
	}

	// Tags
	if len(instance.Tags) > 0 {
		if tagString := buildLightsailTagString(instance.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}

func (l *lightsailProvider) getLightsailClients(region *string) []*lightsail.Lightsail {
	endpoint := fmt.Sprintf("https://lightsail.%s.amazonaws.com", aws.StringValue(region))
	lightsailClients := make([]*lightsail.Lightsail, 0)

	lightsailClient := lightsail.New(
		l.session,
		aws.NewConfig().WithEndpoint(endpoint),
		aws.NewConfig().WithRegion(aws.StringValue(region)),
	)
	lightsailClients = append(lightsailClients, lightsailClient)

	if l.options.AssumeRoleName == "" || len(l.options.AccountIds) < 1 {
		return lightsailClients
	}

	for _, accountId := range l.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, l.options.AssumeRoleName)
		creds := stscreds.NewCredentials(l.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      region,
			Credentials: creds,
		})
		if err != nil {
			continue
		}

		lightsailClients = append(lightsailClients, lightsail.New(assumeSession))
	}
	return lightsailClients
}

func buildLightsailTagString(tags []*lightsail.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
