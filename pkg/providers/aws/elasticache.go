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
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// elastiCacheProvider is a provider for AWS ElastiCache API.
type elastiCacheProvider struct {
	options           ProviderOptions
	elastiCacheClient *elasticache.ElastiCache
	session           *session.Session
	regions           *ec2.DescribeRegionsOutput
}

func (ep *elastiCacheProvider) name() string {
	return "elasticache"
}

// GetResource returns all the resources in the store for a provider.
func (ep *elastiCacheProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, region := range ep.regions.Regions {
		for _, client := range ep.getElastiCacheClients(region.RegionName) {
			wg.Add(1)

			go func(client *elasticache.ElastiCache) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						mu.Lock()
						errs = append(errs, fmt.Errorf("panic in elasticache provider: %v", r))
						mu.Unlock()
					}
				}()

				resources, err := ep.listElastiCacheResources(client)
				mu.Lock()
				defer mu.Unlock()
				if resources != nil {
					list.Merge(resources)
				}
				if err != nil {
					errs = append(errs, err)
				}
			}(client)
		}
	}
	wg.Wait()
	if len(errs) > 0 && len(list.Items) == 0 {
		return nil, fmt.Errorf("elasticache: all workers failed: %v", errs)
	}
	if len(errs) > 0 {
		gologger.Warning().Msgf("elasticache: some listings failed: %v", errs)
	}
	return list, nil
}

func (ep *elastiCacheProvider) listElastiCacheResources(client *elasticache.ElastiCache) (*schema.Resources, error) {
	list := schema.NewResources()
	appendEndpoint := func(endpoint *elasticache.Endpoint, metadata map[string]string) {
		if endpoint == nil || aws.StringValue(endpoint.Address) == "" {
			return
		}
		list.Append(&schema.Resource{
			ID:       ep.options.Id,
			Provider: providerName,
			DNSName:  aws.StringValue(endpoint.Address),
			Public:   true,
			Service:  ep.name(),
			Metadata: metadata,
		})
	}

	err := client.DescribeReplicationGroupsPages(&elasticache.DescribeReplicationGroupsInput{}, func(page *elasticache.DescribeReplicationGroupsOutput, _ bool) bool {
		for _, group := range page.ReplicationGroups {
			var metadata map[string]string
			if ep.options.ExtendedMetadata {
				metadata = getReplicationGroupMetadata(group)
			}
			appendEndpoint(group.ConfigurationEndpoint, metadata)
			for _, nodeGroup := range group.NodeGroups {
				appendEndpoint(nodeGroup.PrimaryEndpoint, metadata)
				appendEndpoint(nodeGroup.ReaderEndpoint, metadata)
				for _, member := range nodeGroup.NodeGroupMembers {
					appendEndpoint(member.ReadEndpoint, metadata)
				}
			}
		}
		return true
	})
	if err != nil {
		return list, errors.Wrap(err, "could not describe elasticache replication groups")
	}

	// Node endpoints are only returned when ShowCacheNodeInfo is set; Memcached
	// clients connect to them directly, so they are reachable endpoints too.
	err = client.DescribeCacheClustersPages(&elasticache.DescribeCacheClustersInput{ShowCacheNodeInfo: aws.Bool(true)}, func(page *elasticache.DescribeCacheClustersOutput, _ bool) bool {
		for _, cluster := range page.CacheClusters {
			var metadata map[string]string
			if ep.options.ExtendedMetadata {
				metadata = getCacheClusterMetadata(cluster)
			}
			appendEndpoint(cluster.ConfigurationEndpoint, metadata)
			for _, node := range cluster.CacheNodes {
				appendEndpoint(node.Endpoint, metadata)
			}
		}
		return true
	})
	if err != nil {
		return list, errors.Wrap(err, "could not describe elasticache cache clusters")
	}

	// Serverless caches are not available in every region, so a failure here
	// must not discard the clusters already found.
	_ = client.DescribeServerlessCachesPages(&elasticache.DescribeServerlessCachesInput{}, func(page *elasticache.DescribeServerlessCachesOutput, _ bool) bool {
		for _, cache := range page.ServerlessCaches {
			var metadata map[string]string
			if ep.options.ExtendedMetadata {
				metadata = getServerlessCacheMetadata(cache)
			}
			appendEndpoint(cache.Endpoint, metadata)
			appendEndpoint(cache.ReaderEndpoint, metadata)
		}
		return true
	})

	return list, nil
}

func getReplicationGroupMetadata(group *elasticache.ReplicationGroup) map[string]string {
	metadata := make(map[string]string)
	schema.AddMetadata(metadata, "replication_group_id", group.ReplicationGroupId)
	schema.AddMetadata(metadata, "arn", group.ARN)
	schema.AddMetadata(metadata, "status", group.Status)
	schema.AddMetadata(metadata, "cache_node_type", group.CacheNodeType)
	schema.AddMetadata(metadata, "cluster_mode", group.ClusterMode)
	if group.TransitEncryptionEnabled != nil {
		metadata["transit_encryption_enabled"] = fmt.Sprintf("%t", *group.TransitEncryptionEnabled)
	}
	if group.AuthTokenEnabled != nil {
		metadata["auth_token_enabled"] = fmt.Sprintf("%t", *group.AuthTokenEnabled)
	}
	return metadata
}

func getCacheClusterMetadata(cluster *elasticache.CacheCluster) map[string]string {
	metadata := make(map[string]string)
	schema.AddMetadata(metadata, "cache_cluster_id", cluster.CacheClusterId)
	schema.AddMetadata(metadata, "arn", cluster.ARN)
	schema.AddMetadata(metadata, "replication_group_id", cluster.ReplicationGroupId)
	schema.AddMetadata(metadata, "engine", cluster.Engine)
	schema.AddMetadata(metadata, "engine_version", cluster.EngineVersion)
	schema.AddMetadata(metadata, "status", cluster.CacheClusterStatus)
	schema.AddMetadata(metadata, "cache_node_type", cluster.CacheNodeType)
	schema.AddMetadata(metadata, "availability_zone", cluster.PreferredAvailabilityZone)
	if cluster.TransitEncryptionEnabled != nil {
		metadata["transit_encryption_enabled"] = fmt.Sprintf("%t", *cluster.TransitEncryptionEnabled)
	}
	if cluster.CacheClusterCreateTime != nil {
		metadata["created_at"] = cluster.CacheClusterCreateTime.Format(time.RFC3339)
	}
	return metadata
}

func getServerlessCacheMetadata(cache *elasticache.ServerlessCache) map[string]string {
	metadata := make(map[string]string)
	schema.AddMetadata(metadata, "serverless_cache_name", cache.ServerlessCacheName)
	schema.AddMetadata(metadata, "arn", cache.ARN)
	schema.AddMetadata(metadata, "engine", cache.Engine)
	schema.AddMetadata(metadata, "engine_version", cache.FullEngineVersion)
	schema.AddMetadata(metadata, "status", cache.Status)
	if len(cache.SecurityGroupIds) > 0 {
		metadata["security_group_ids"] = strings.Join(aws.StringValueSlice(cache.SecurityGroupIds), ",")
	}
	if cache.CreateTime != nil {
		metadata["created_at"] = cache.CreateTime.Format(time.RFC3339)
	}
	return metadata
}

func (ep *elastiCacheProvider) getElastiCacheClients(region *string) []*elasticache.ElastiCache {
	clients := make([]*elasticache.ElastiCache, 0)

	clients = append(clients, elasticache.New(
		ep.session,
		aws.NewConfig().WithRegion(aws.StringValue(region)),
	))

	if ep.options.AssumeRoleName == "" || len(ep.options.AccountIds) < 1 {
		return clients
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

		clients = append(clients, elasticache.New(assumeSession))
	}
	return clients
}
