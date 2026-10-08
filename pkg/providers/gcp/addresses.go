package gcp

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/compute/v1"
)

// getAddressResources lists reserved addresses and forwarding rules (load
// balancer frontends), which are not attached to instances and so are missed
// by the instance listing. Internal IPs are classified as private on Append.
func (d *cloudVMProvider) getAddressResources(ctx context.Context, project string) *schema.Resources {
	list := schema.NewResources()

	appendAddress := func(address *compute.Address) {
		if address == nil || address.Address == "" {
			return
		}
		var metadata map[string]string
		if d.extendedMetadata {
			metadata = getAddressMetadata(address, project)
		}
		list.Append(&schema.Resource{
			ID:         d.id,
			Public:     true,
			Provider:   providerName,
			PublicIPv4: address.Address,
			Service:    d.name(),
			Metadata:   metadata,
		})
	}

	appendForwardingRule := func(rule *compute.ForwardingRule) {
		if rule == nil || rule.IPAddress == "" {
			return
		}
		var metadata map[string]string
		if d.extendedMetadata {
			metadata = getForwardingRuleMetadata(rule, project)
		}
		list.Append(&schema.Resource{
			ID:         d.id,
			Public:     true,
			Provider:   providerName,
			PublicIPv4: rule.IPAddress,
			Service:    d.name(),
			Metadata:   metadata,
		})
	}

	err := d.compute.Addresses.AggregatedList(project).Pages(ctx, func(resp *compute.AddressAggregatedList) error {
		for _, scoped := range resp.Items {
			for _, address := range scoped.Addresses {
				appendAddress(address)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Could not get all addresses for project %s: %s\n", project, err)
	}

	err = d.compute.GlobalAddresses.List(project).Pages(ctx, func(resp *compute.AddressList) error {
		for _, address := range resp.Items {
			appendAddress(address)
		}
		return nil
	})
	if err != nil {
		log.Printf("Could not get all global addresses for project %s: %s\n", project, err)
	}

	err = d.compute.ForwardingRules.AggregatedList(project).Pages(ctx, func(resp *compute.ForwardingRuleAggregatedList) error {
		for _, scoped := range resp.Items {
			for _, rule := range scoped.ForwardingRules {
				appendForwardingRule(rule)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Could not get all forwarding rules for project %s: %s\n", project, err)
	}

	err = d.compute.GlobalForwardingRules.List(project).Pages(ctx, func(resp *compute.ForwardingRuleList) error {
		for _, rule := range resp.Items {
			appendForwardingRule(rule)
		}
		return nil
	})
	if err != nil {
		log.Printf("Could not get all global forwarding rules for project %s: %s\n", project, err)
	}

	return list
}

func getAddressMetadata(address *compute.Address, project string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "address_name", &address.Name)
	if address.Id != 0 {
		metadata["address_id"] = fmt.Sprintf("%d", address.Id)
	}
	schema.AddMetadata(metadata, "address_type", &address.AddressType)
	schema.AddMetadata(metadata, "status", &address.Status)
	schema.AddMetadata(metadata, "purpose", &address.Purpose)
	schema.AddMetadata(metadata, "network_tier", &address.NetworkTier)
	schema.AddMetadata(metadata, "ip_version", &address.IpVersion)
	region := extractResourceName(address.Region)
	schema.AddMetadata(metadata, "region", &region)
	schema.AddMetadata(metadata, "creation_timestamp", &address.CreationTimestamp)
	schema.AddMetadata(metadata, "description", &address.Description)

	if len(address.Users) > 0 {
		users := make([]string, 0, len(address.Users))
		for _, user := range address.Users {
			users = append(users, extractResourceName(user))
		}
		metadata["users"] = strings.Join(users, ",")
	}
	if len(address.Labels) > 0 {
		metadata["labels"] = joinLabels(address.Labels)
	}

	metadata["project_id"] = project
	metadata["owner_id"] = project
	return metadata
}

func getForwardingRuleMetadata(rule *compute.ForwardingRule, project string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "forwarding_rule_name", &rule.Name)
	if rule.Id != 0 {
		metadata["forwarding_rule_id"] = fmt.Sprintf("%d", rule.Id)
	}
	schema.AddMetadata(metadata, "load_balancing_scheme", &rule.LoadBalancingScheme)
	schema.AddMetadata(metadata, "ip_protocol", &rule.IPProtocol)
	schema.AddMetadata(metadata, "port_range", &rule.PortRange)
	if len(rule.Ports) > 0 {
		metadata["ports"] = strings.Join(rule.Ports, ",")
	}
	schema.AddMetadata(metadata, "network_tier", &rule.NetworkTier)
	target := extractResourceName(rule.Target)
	schema.AddMetadata(metadata, "target", &target)
	backendService := extractResourceName(rule.BackendService)
	schema.AddMetadata(metadata, "backend_service", &backendService)
	region := extractResourceName(rule.Region)
	schema.AddMetadata(metadata, "region", &region)
	schema.AddMetadata(metadata, "creation_timestamp", &rule.CreationTimestamp)
	schema.AddMetadata(metadata, "description", &rule.Description)

	if len(rule.Labels) > 0 {
		metadata["labels"] = joinLabels(rule.Labels)
	}

	metadata["project_id"] = project
	metadata["owner_id"] = project
	return metadata
}

func joinLabels(labels map[string]string) string {
	pairs := make([]string, 0, len(labels))
	for key, value := range labels {
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(pairs, ",")
}
