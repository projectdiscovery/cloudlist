package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// getElasticIPResources lists Elastic IPs, including ones not attached to an
// instance (e.g. reserved, or bound to a NAT gateway or bare ENI), which
// DescribeInstances never returns.
func (i *instanceProvider) getElasticIPResources(ec2Client *ec2.EC2) (*schema.Resources, error) {
	list := schema.NewResources()

	// DescribeAddresses is not paginated.
	resp, err := ec2Client.DescribeAddresses(&ec2.DescribeAddressesInput{})
	if err != nil {
		return nil, err
	}

	for _, address := range resp.Addresses {
		if address == nil || aws.StringValue(address.PublicIp) == "" {
			continue
		}

		var metadata map[string]string
		if i.options.ExtendedMetadata {
			metadata = getElasticIPMetadata(address)
		}

		list.Append(&schema.Resource{
			ID:         i.options.Id,
			Provider:   providerName,
			PublicIPv4: aws.StringValue(address.PublicIp),
			Public:     true,
			Service:    "eip",
			Metadata:   metadata,
		})
	}
	return list, nil
}

func getElasticIPMetadata(address *ec2.Address) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "allocation_id", address.AllocationId)
	schema.AddMetadata(metadata, "association_id", address.AssociationId)
	schema.AddMetadata(metadata, "instance_id", address.InstanceId)
	schema.AddMetadata(metadata, "network_interface_id", address.NetworkInterfaceId)
	schema.AddMetadata(metadata, "private_ip_address", address.PrivateIpAddress)
	schema.AddMetadata(metadata, "domain", address.Domain)
	schema.AddMetadata(metadata, "public_ipv4_pool", address.PublicIpv4Pool)
	schema.AddMetadata(metadata, "network_border_group", address.NetworkBorderGroup)

	if len(address.Tags) > 0 {
		if tagString := buildTagString(address.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}
	return metadata
}
