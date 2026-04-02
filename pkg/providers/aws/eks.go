package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
)

// eksProvider is a provider for AWS EKS API.
type eksProvider struct {
	options   ProviderOptions
	eksClient *eks.EKS
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

func (ep *eksProvider) name() string {
	return "eks"
}

// GetResource returns all the resources in the store for a provider.
func (ep *eksProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for _, region := range ep.regions.Regions {
		for _, eksClient := range ep.getEksClients(region.RegionName) {
			wg.Add(1)

			go func(client *eks.EKS) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						mu.Lock()
						errs = append(errs, fmt.Errorf("panic in eks provider: %v", r))
						mu.Unlock()
					}
				}()
				if resources, err := ep.listEKSResources(client); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(eksClient)
		}
	}
	wg.Wait()
	if len(errs) > 0 && len(list.Items) == 0 {
		return nil, fmt.Errorf("eks: all workers failed: %v", errs)
	}
	return list, nil
}

func (ep *eksProvider) listEKSResources(eksClient *eks.EKS) (*schema.Resources, error) {
	list := schema.NewResources()
	req := &eks.ListClustersInput{
		MaxResults: aws.Int64(100),
	}
	for {
		clustersOutput, err := eksClient.ListClusters(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not list EKS clusters")
		}
		for _, clusterName := range clustersOutput.Clusters {
			clusterOutput, err := eksClient.DescribeCluster(&eks.DescribeClusterInput{
				Name: clusterName,
			})
			if err != nil {
				return nil, errors.Wrapf(err, "could not describe EKS cluster: %s", *clusterName)
			}

			var clusterMetadata map[string]string
			if ep.options.ExtendedMetadata {
				clusterMetadata = ep.getClusterMetadata(clusterOutput.Cluster, eksClient)
			}

			clientset, err := newClientset(clusterOutput.Cluster)
			if err != nil {
				return nil, errors.Wrapf(err, "could not create clientset for EKS cluster: %s", *clusterName)
			}
			nodes, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
			if err != nil {
				return nil, errors.Wrapf(err, "could not list nodes for EKS cluster: %s", *clusterName)
			}
			for _, node := range nodes.Items {
				var nodeMetadata map[string]string
				if ep.options.ExtendedMetadata {
					nodeMetadata = ep.getNodeMetadata(&node, clusterOutput.Cluster)
				}

				var podIPs []string
				pods, err := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{
					FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.GetName()),
				})
				if err != nil {
					continue
				}
				for _, pod := range pods.Items {
					for _, podIP := range pod.Status.PodIPs {
						podIPs = append(podIPs, podIP.IP)
					}
				}
				nodeIP := node.Status.Addresses[0].Address
				// Generate unique ID for node using cluster name and node name
				nodeID := fmt.Sprintf("%s-%s-%s", ep.options.Id, aws.StringValue(clusterName), node.GetName())
				list.Append(&schema.Resource{
					Provider:   providerName,
					ID:         nodeID,
					PublicIPv4: nodeIP,
					Public:     true,
					Service:    ep.name(),
					Metadata:   nodeMetadata,
				})
				for _, podIP := range podIPs {
					var podMetadata map[string]string
					if ep.options.ExtendedMetadata && clusterMetadata != nil {
						podMetadata = make(map[string]string)
						podMetadata["cluster_name"] = clusterMetadata["cluster_name"]
						podMetadata["cluster_arn"] = clusterMetadata["cluster_arn"]
						podMetadata["owner_id"] = clusterMetadata["owner_id"]
						podMetadata["node_name"] = node.GetName()
					}
					// Generate unique ID for pod using cluster name, node name, and pod IP
					podID := fmt.Sprintf("%s-%s-%s-%s", ep.options.Id, aws.StringValue(clusterName), node.GetName(), strings.ReplaceAll(podIP, ".", "-"))
					list.Append(&schema.Resource{
						Provider:    providerName,
						ID:          podID,
						PrivateIpv4: podIP,
						Public:      false,
						Service:     ep.name(),
						Metadata:    podMetadata,
					})
				}
			}
		}
		if aws.StringValue(clustersOutput.NextToken) == "" {
			break
		}
		req.SetNextToken(*clustersOutput.NextToken)
	}
	return list, nil
}

func (ep *eksProvider) getClusterMetadata(cluster *eks.Cluster, eksClient *eks.EKS) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "cluster_name", cluster.Name)
	schema.AddMetadata(metadata, "cluster_arn", cluster.Arn)
	schema.AddMetadata(metadata, "version", cluster.Version)
	schema.AddMetadata(metadata, "platform_version", cluster.PlatformVersion)
	schema.AddMetadata(metadata, "status", cluster.Status)
	schema.AddMetadata(metadata, "endpoint", cluster.Endpoint)

	if cluster.Arn != nil {
		if arnComponents := parseARN(aws.StringValue(cluster.Arn)); arnComponents != nil && arnComponents.AccountID != "" {
			metadata["owner_id"] = arnComponents.AccountID
		}
	}

	if cluster.CreatedAt != nil {
		metadata["created_at"] = cluster.CreatedAt.Format(time.RFC3339)
	}

	schema.AddMetadata(metadata, "role_arn", cluster.RoleArn)

	if cluster.ResourcesVpcConfig != nil {
		schema.AddMetadata(metadata, "vpc_id", cluster.ResourcesVpcConfig.VpcId)

		if len(cluster.ResourcesVpcConfig.PublicAccessCidrs) > 0 {
			metadata["public_access_cidrs"] = strings.Join(aws.StringValueSlice(cluster.ResourcesVpcConfig.PublicAccessCidrs), ",")
		}

		if cluster.ResourcesVpcConfig.EndpointPublicAccess != nil {
			metadata["endpoint_public_access"] = fmt.Sprintf("%v", aws.BoolValue(cluster.ResourcesVpcConfig.EndpointPublicAccess))
		}

		if cluster.ResourcesVpcConfig.EndpointPrivateAccess != nil {
			metadata["endpoint_private_access"] = fmt.Sprintf("%v", aws.BoolValue(cluster.ResourcesVpcConfig.EndpointPrivateAccess))
		}

		if len(cluster.ResourcesVpcConfig.SubnetIds) > 0 {
			metadata["subnet_ids"] = strings.Join(aws.StringValueSlice(cluster.ResourcesVpcConfig.SubnetIds), ",")
		}

		if len(cluster.ResourcesVpcConfig.SecurityGroupIds) > 0 {
			metadata["security_group_ids"] = strings.Join(aws.StringValueSlice(cluster.ResourcesVpcConfig.SecurityGroupIds), ",")
		}
	}

	if cluster.Identity != nil && cluster.Identity.Oidc != nil {
		schema.AddMetadata(metadata, "oidc_issuer", cluster.Identity.Oidc.Issuer)
	}

	if cluster.CertificateAuthority != nil {
		if cluster.CertificateAuthority.Data != nil {
			metadata["has_certificate_authority"] = "true"
		}
	}

	if cluster.Arn != nil {
		if tagOutput, err := eksClient.ListTagsForResource(&eks.ListTagsForResourceInput{
			ResourceArn: cluster.Arn,
		}); err == nil && tagOutput.Tags != nil {
			if tagString := buildAwsMapTagString(tagOutput.Tags); tagString != "" {
				metadata["tags"] = tagString
			}
		}
	}

	return metadata
}

func (ep *eksProvider) getNodeMetadata(node *corev1.Node, cluster *eks.Cluster) map[string]string {
	metadata := make(map[string]string)

	metadata["node_name"] = node.GetName()

	schema.AddMetadata(metadata, "cluster_name", cluster.Name)
	if cluster.Arn != nil {
		if arnComponents := parseARN(aws.StringValue(cluster.Arn)); arnComponents != nil && arnComponents.AccountID != "" {
			metadata["owner_id"] = arnComponents.AccountID
		}
	}

	if node.CreationTimestamp.Time != (time.Time{}) {
		metadata["node_created_at"] = node.CreationTimestamp.Format(time.RFC3339)
	}

	if len(node.Labels) > 0 {
		var labelPairs []string
		if instanceType, ok := node.Labels["node.kubernetes.io/instance-type"]; ok {
			metadata["instance_type"] = instanceType
		}
		if zone, ok := node.Labels["topology.kubernetes.io/zone"]; ok {
			metadata["availability_zone"] = zone
		}
		if nodeGroup, ok := node.Labels["eks.amazonaws.com/nodegroup"]; ok {
			metadata["nodegroup"] = nodeGroup
		}
		if capacityType, ok := node.Labels["eks.amazonaws.com/capacityType"]; ok {
			metadata["capacity_type"] = capacityType
		}

		for k, v := range node.Labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
		}
		if len(labelPairs) > 0 {
			metadata["labels"] = strings.Join(labelPairs, ",")
		}
	}

	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" {
			metadata["node_ready"] = string(condition.Status)
			break
		}
	}

	if node.Status.Capacity != nil {
		if cpu, ok := node.Status.Capacity["cpu"]; ok {
			metadata["cpu_capacity"] = cpu.String()
		}
		if memory, ok := node.Status.Capacity["memory"]; ok {
			metadata["memory_capacity"] = memory.String()
		}
		if pods, ok := node.Status.Capacity["pods"]; ok {
			metadata["pods_capacity"] = pods.String()
		}
	}

	if node.Status.NodeInfo.KubeletVersion != "" {
		metadata["kubelet_version"] = node.Status.NodeInfo.KubeletVersion
	}
	if node.Status.NodeInfo.ContainerRuntimeVersion != "" {
		metadata["container_runtime"] = node.Status.NodeInfo.ContainerRuntimeVersion
	}
	if node.Status.NodeInfo.OSImage != "" {
		metadata["os_image"] = node.Status.NodeInfo.OSImage
	}
	if node.Status.NodeInfo.Architecture != "" {
		metadata["architecture"] = node.Status.NodeInfo.Architecture
	}

	if node.Spec.ProviderID != "" {
		metadata["provider_id"] = node.Spec.ProviderID
		parts := strings.Split(node.Spec.ProviderID, "/")
		if len(parts) > 0 {
			metadata["instance_id"] = parts[len(parts)-1]
		}
	}

	for _, addr := range node.Status.Addresses {
		switch addr.Type {
		case "InternalIP":
			metadata["internal_ip"] = addr.Address
		case "ExternalIP":
			metadata["external_ip"] = addr.Address
		case "InternalDNS":
			metadata["internal_dns"] = addr.Address
		case "ExternalDNS":
			metadata["external_dns"] = addr.Address
		case "Hostname":
			metadata["hostname"] = addr.Address
		}
	}

	return metadata
}

func (ep *eksProvider) getEksClients(region *string) []*eks.EKS {
	eksClients := make([]*eks.EKS, 0)

	eksClient := eks.New(
		ep.session,
		aws.NewConfig().WithRegion(aws.StringValue(region)),
	)
	eksClients = append(eksClients, eksClient)

	if ep.options.AssumeRoleName == "" || len(ep.options.AccountIds) < 1 {
		return eksClients
	}

	for _, accountId := range ep.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, ep.options.AssumeRoleName)
		creds := stscreds.NewCredentials(ep.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      region,
			Credentials: creds,
		})
		if err != nil {
			continue
		}

		eksClients = append(eksClients, eks.New(assumeSession))
	}
	return eksClients
}

func newClientset(cluster *eks.Cluster) (*kubernetes.Clientset, error) {
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, err
	}
	opts := &token.GetTokenOptions{
		ClusterID: aws.StringValue(cluster.Name),
	}
	tok, err := gen.GetWithOptions(opts)
	if err != nil {
		return nil, err
	}
	ca, err := base64.StdEncoding.DecodeString(aws.StringValue(cluster.CertificateAuthority.Data))
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(
		&rest.Config{
			Host:        aws.StringValue(cluster.Endpoint),
			BearerToken: tok.Token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: ca,
			},
		},
	)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}
