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
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// awsInstanceProvider is an instance provider for aws API
type instanceProvider struct {
	options   ProviderOptions
	ec2Client *ec2.EC2
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetResource returns all the resources in the store for a provider.
func (i *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	totalGoroutines := 0
	for _, region := range i.regions.Regions {
		clients := i.getEc2Clients(region.RegionName)
		fmt.Printf("[cloudlist-debug] ec2: region=%s clients=%d (1 base + %d assumed)\n", aws.StringValue(region.RegionName), len(clients), len(clients)-1)
		for _, ec2Client := range clients {
			wg.Add(1)
			totalGoroutines++

			go func(ec2Client *ec2.EC2, regionName string) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						gologger.Error().Msgf("panic in %s provider goroutine: %v", "ec2", r)
					}
				}()
				start := time.Now()
				resources, err := i.getEC2Resources(ec2Client)
				fmt.Printf("[cloudlist-debug] ec2: region=%s took=%v err=%v items=%d\n", regionName, time.Since(start), err, func() int { if resources != nil { return len(resources.Items) }; return 0 }())
				if err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(ec2Client, aws.StringValue(region.RegionName))
		}
	}
	fmt.Printf("[cloudlist-debug] ec2: waiting for %d goroutines\n", totalGoroutines)
	wg.Wait()
	fmt.Printf("[cloudlist-debug] ec2: all goroutines done, total items=%d\n", len(list.Items))
	return list, nil
}

func (i *instanceProvider) getEC2Resources(ec2Client *ec2.EC2) (*schema.Resources, error) {
	list := schema.NewResources()

	req := &ec2.DescribeInstancesInput{
		MaxResults: aws.Int64(1000),
	}
	for {
		resp, err := ec2Client.DescribeInstances(req)
		if err != nil {
			return nil, err
		}

		for _, reservation := range resp.Reservations {
			for _, instance := range reservation.Instances {
				ip4 := aws.StringValue(instance.PublicIpAddress)
				ip6 := aws.StringValue(instance.Ipv6Address)
				privateIp4 := aws.StringValue(instance.PrivateIpAddress)

				// Extract metadata for this instance
				var metadata map[string]string
				if i.options.ExtendedMetadata {
					metadata = i.getInstanceMetadata(instance, reservation)
				}

				if privateIp4 != "" {
					list.Append(&schema.Resource{
						ID:          i.options.Id,
						Provider:    providerName,
						PrivateIpv4: privateIp4,
						Public:      false,
						Service:     i.name(),
						Metadata:    metadata,
					})
				}
				list.Append(&schema.Resource{
					ID:         i.options.Id,
					Provider:   providerName,
					PublicIPv4: ip4,
					PublicIPv6: ip6,
					Public:     true,
					Service:    i.name(),
					Metadata:   metadata,
				})
			}
		}
		if aws.StringValue(resp.NextToken) == "" {
			break
		}
		req.SetNextToken(aws.StringValue(resp.NextToken))
	}
	return list, nil
}

func (i *instanceProvider) getInstanceMetadata(instance *ec2.Instance, reservation *ec2.Reservation) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "instance_id", instance.InstanceId)
	schema.AddMetadata(metadata, "instance_type", instance.InstanceType)
	schema.AddMetadata(metadata, "ami_id", instance.ImageId)
	schema.AddMetadata(metadata, "architecture", instance.Architecture)
	schema.AddMetadata(metadata, "platform_details", instance.PlatformDetails)
	schema.AddMetadata(metadata, "owner_id", reservation.OwnerId)
	schema.AddMetadata(metadata, "key_name", instance.KeyName)

	if instance.State != nil {
		schema.AddMetadata(metadata, "instance_state", instance.State.Name)
	}

	if len(instance.SecurityGroups) > 0 {
		var sgIds []string
		var sgNames []string
		for _, sg := range instance.SecurityGroups {
			if sg.GroupId != nil {
				sgIds = append(sgIds, aws.StringValue(sg.GroupId))
			}
			if sg.GroupName != nil {
				sgNames = append(sgNames, aws.StringValue(sg.GroupName))
			}
		}
		if len(sgIds) > 0 {
			metadata["security_group_ids"] = strings.Join(sgIds, ",")
		}
		if len(sgNames) > 0 {
			metadata["security_group_names"] = strings.Join(sgNames, ",")
		}
	}

	schema.AddMetadata(metadata, "vpc_id", instance.VpcId)
	schema.AddMetadata(metadata, "subnet_id", instance.SubnetId)

	if instance.Placement != nil {
		schema.AddMetadata(metadata, "availability_zone", instance.Placement.AvailabilityZone)
	}

	schema.AddMetadata(metadata, "private_dns_name", instance.PrivateDnsName)
	schema.AddMetadata(metadata, "public_dns_name", instance.PublicDnsName)

	if instance.LaunchTime != nil {
		metadata["launch_time"] = instance.LaunchTime.Format(time.RFC3339)
	}

	if instance.Monitoring != nil {
		schema.AddMetadata(metadata, "monitoring_state", instance.Monitoring.State)
	}

	if len(instance.Tags) > 0 {
		if tagString := buildTagString(instance.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	// IAM Instance Profile (often contains role/ownership information)
	if instance.IamInstanceProfile != nil {
		schema.AddMetadata(metadata, "iam_instance_profile", instance.IamInstanceProfile.Arn)
	}

	return metadata
}

func (i *instanceProvider) getEc2Clients(region *string) []*ec2.EC2 {
	endpoint := fmt.Sprintf("https://ec2.%s.amazonaws.com", aws.StringValue(region))
	ec2Clients := make([]*ec2.EC2, 0)

	ec2Client := ec2.New(
		i.session,
		aws.NewConfig().WithEndpoint(endpoint),
		aws.NewConfig().WithRegion(aws.StringValue(region)),
	)
	ec2Clients = append(ec2Clients, ec2Client)

	if i.options.AssumeRoleName == "" || len(i.options.AccountIds) < 1 {
		return ec2Clients
	}

	for _, accountId := range i.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, i.options.AssumeRoleName)
		creds := stscreds.NewCredentials(i.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      region,
			Credentials: creds,
		})
		if err != nil {
			continue
		}

		ec2Clients = append(ec2Clients, ec2.New(assumeSession))
	}
	return ec2Clients
}

func buildTagString(tags []*ec2.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s",
				aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
