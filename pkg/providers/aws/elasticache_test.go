package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var elastiCacheResponses = map[string]string{
	"DescribeReplicationGroups": `<DescribeReplicationGroupsResponse xmlns="http://elasticache.amazonaws.com/doc/2015-02-02/">
  <DescribeReplicationGroupsResult>
    <ReplicationGroups>
      <ReplicationGroup>
        <ReplicationGroupId>redis-rg</ReplicationGroupId>
        <Status>available</Status>
        <NodeGroups>
          <NodeGroup>
            <PrimaryEndpoint><Address>redis-rg.abc123.ng.0001.use1.cache.amazonaws.com</Address><Port>6379</Port></PrimaryEndpoint>
            <ReaderEndpoint><Address>redis-rg-ro.abc123.ng.0001.use1.cache.amazonaws.com</Address><Port>6379</Port></ReaderEndpoint>
            <NodeGroupMembers>
              <NodeGroupMember>
                <CacheClusterId>redis-rg-001</CacheClusterId>
                <ReadEndpoint><Address>redis-rg-001.abc123.0001.use1.cache.amazonaws.com</Address><Port>6379</Port></ReadEndpoint>
              </NodeGroupMember>
            </NodeGroupMembers>
          </NodeGroup>
        </NodeGroups>
      </ReplicationGroup>
      <ReplicationGroup>
        <ReplicationGroupId>redis-cluster-mode</ReplicationGroupId>
        <ConfigurationEndpoint><Address>clustercfg.redis-cluster-mode.abc123.use1.cache.amazonaws.com</Address><Port>6379</Port></ConfigurationEndpoint>
      </ReplicationGroup>
    </ReplicationGroups>
  </DescribeReplicationGroupsResult>
</DescribeReplicationGroupsResponse>`,
	"DescribeCacheClusters": `<DescribeCacheClustersResponse xmlns="http://elasticache.amazonaws.com/doc/2015-02-02/">
  <DescribeCacheClustersResult>
    <CacheClusters>
      <CacheCluster>
        <CacheClusterId>redis-rg-001</CacheClusterId>
        <ReplicationGroupId>redis-rg</ReplicationGroupId>
        <Engine>redis</Engine>
        <CacheNodes>
          <CacheNode><CacheNodeId>0001</CacheNodeId><Endpoint><Address>redis-rg-001.abc123.0001.use1.cache.amazonaws.com</Address><Port>6379</Port></Endpoint></CacheNode>
        </CacheNodes>
      </CacheCluster>
      <CacheCluster>
        <CacheClusterId>memcached</CacheClusterId>
        <Engine>memcached</Engine>
        <ConfigurationEndpoint><Address>memcached.abc123.cfg.use1.cache.amazonaws.com</Address><Port>11211</Port></ConfigurationEndpoint>
        <CacheNodes>
          <CacheNode><CacheNodeId>0001</CacheNodeId><Endpoint><Address>memcached.abc123.0001.use1.cache.amazonaws.com</Address><Port>11211</Port></Endpoint></CacheNode>
          <CacheNode><CacheNodeId>0002</CacheNodeId></CacheNode>
        </CacheNodes>
      </CacheCluster>
    </CacheClusters>
  </DescribeCacheClustersResult>
</DescribeCacheClustersResponse>`,
	"DescribeServerlessCaches": `<DescribeServerlessCachesResponse xmlns="http://elasticache.amazonaws.com/doc/2015-02-02/">
  <DescribeServerlessCachesResult>
    <ServerlessCaches>
      <member>
        <ServerlessCacheName>valkey-serverless</ServerlessCacheName>
        <Engine>valkey</Engine>
        <Endpoint><Address>valkey-serverless-abc123.serverless.use1.cache.amazonaws.com</Address><Port>6379</Port></Endpoint>
        <ReaderEndpoint><Address>valkey-serverless-abc123.serverless.use1.cache.amazonaws.com</Address><Port>6380</Port></ReaderEndpoint>
      </member>
    </ServerlessCaches>
  </DescribeServerlessCachesResult>
</DescribeServerlessCachesResponse>`,
}

func newTestElastiCacheClient(t *testing.T, serverlessStatus, cacheStatus int) *elasticache.ElastiCache {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		action := r.Form.Get("Action")
		if action == "DescribeCacheClusters" {
			assert.Equal(t, "true", r.Form.Get("ShowCacheNodeInfo"))
		}
		w.Header().Set("Content-Type", "text/xml")
		if action == "DescribeCacheClusters" && cacheStatus != http.StatusOK {
			w.WriteHeader(cacheStatus)
			_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>AccessDenied</Code><Message>denied</Message></Error></ErrorResponse>`))
			return
		}
		if action == "DescribeServerlessCaches" && serverlessStatus != http.StatusOK {
			w.WriteHeader(serverlessStatus)
			_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>InvalidParameterValue</Code><Message>not supported</Message></Error></ErrorResponse>`))
			return
		}
		body, ok := elastiCacheResponses[action]
		require.True(t, ok, "unexpected action %s", action)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(server.URL),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
		MaxRetries:  aws.Int(0),
	})
	require.NoError(t, err)
	return elasticache.New(sess)
}

func dnsNames(t *testing.T, provider *elastiCacheProvider, client *elasticache.ElastiCache) []string {
	t.Helper()
	resources, err := provider.listElastiCacheResources(client)
	require.NoError(t, err)

	var names []string
	for _, item := range resources.Items {
		assert.Equal(t, "elasticache", item.Service)
		names = append(names, item.DNSName)
	}
	sort.Strings(names)
	return names
}

func TestListElastiCacheResources(t *testing.T) {
	provider := &elastiCacheProvider{options: ProviderOptions{Id: "test"}}

	assert.Equal(t, []string{
		"clustercfg.redis-cluster-mode.abc123.use1.cache.amazonaws.com",
		"memcached.abc123.0001.use1.cache.amazonaws.com",
		"memcached.abc123.cfg.use1.cache.amazonaws.com",
		"redis-rg-001.abc123.0001.use1.cache.amazonaws.com",
		"redis-rg-ro.abc123.ng.0001.use1.cache.amazonaws.com",
		"redis-rg.abc123.ng.0001.use1.cache.amazonaws.com",
		"valkey-serverless-abc123.serverless.use1.cache.amazonaws.com",
	}, dnsNames(t, provider, newTestElastiCacheClient(t, http.StatusOK, http.StatusOK)))
}

func TestListElastiCacheResources_ServerlessUnsupported(t *testing.T) {
	provider := &elastiCacheProvider{options: ProviderOptions{Id: "test"}}

	names := dnsNames(t, provider, newTestElastiCacheClient(t, http.StatusBadRequest, http.StatusOK))
	assert.Len(t, names, 6, "a serverless API failure must not drop cluster endpoints")
	assert.NotContains(t, names, "valkey-serverless-abc123.serverless.use1.cache.amazonaws.com")
}

func TestListElastiCacheResourcesKeepsEarlierReplicationGroupPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "text/xml")
		if r.Form.Get("Action") == "DescribeReplicationGroups" && r.Form.Get("Marker") == "next" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>InternalError</Code><Message>page failed</Message></Error></ErrorResponse>`))
			return
		}
		if r.Form.Get("Action") != "DescribeReplicationGroups" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>AccessDenied</Code><Message>denied</Message></Error></ErrorResponse>`))
			return
		}
		_, _ = w.Write([]byte(`<DescribeReplicationGroupsResponse xmlns="http://elasticache.amazonaws.com/doc/2015-02-02/">
  <DescribeReplicationGroupsResult>
    <Marker>next</Marker>
    <ReplicationGroups>
      <ReplicationGroup>
        <ReplicationGroupId>redis-rg</ReplicationGroupId>
        <NodeGroups><NodeGroup>
          <PrimaryEndpoint><Address>redis-rg.abc123.ng.0001.use1.cache.amazonaws.com</Address><Port>6379</Port></PrimaryEndpoint>
        </NodeGroup></NodeGroups>
      </ReplicationGroup>
    </ReplicationGroups>
  </DescribeReplicationGroupsResult>
</DescribeReplicationGroupsResponse>`))
	}))
	t.Cleanup(server.Close)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(server.URL),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
		MaxRetries:  aws.Int(0),
	})
	require.NoError(t, err)

	provider := &elastiCacheProvider{options: ProviderOptions{Id: "test"}}
	resources, err := provider.listElastiCacheResources(elasticache.New(sess))
	require.Error(t, err)
	require.NotNil(t, resources)
	var names []string
	for _, item := range resources.Items {
		names = append(names, item.DNSName)
	}
	assert.Equal(t, []string{"redis-rg.abc123.ng.0001.use1.cache.amazonaws.com"}, names)
}

func TestListElastiCacheResourcesKeepsReplicationGroups(t *testing.T) {
	provider := &elastiCacheProvider{options: ProviderOptions{Id: "test"}}
	resources, err := provider.listElastiCacheResources(newTestElastiCacheClient(t, http.StatusOK, http.StatusForbidden))
	require.Error(t, err)

	var names []string
	for _, item := range resources.Items {
		names = append(names, item.DNSName)
	}
	assert.Contains(t, names, "redis-rg.abc123.ng.0001.use1.cache.amazonaws.com")
	assert.NotContains(t, names, "memcached.abc123.cfg.use1.cache.amazonaws.com")
}

func TestListElastiCacheResources_ExtendedMetadata(t *testing.T) {
	provider := &elastiCacheProvider{options: ProviderOptions{Id: "test", ExtendedMetadata: true}}

	resources, err := provider.listElastiCacheResources(newTestElastiCacheClient(t, http.StatusOK, http.StatusOK))
	require.NoError(t, err)

	byName := map[string]map[string]string{}
	for _, item := range resources.Items {
		byName[item.DNSName] = item.Metadata
	}
	assert.Equal(t, "redis-rg", byName["redis-rg.abc123.ng.0001.use1.cache.amazonaws.com"]["replication_group_id"])
	assert.Equal(t, "memcached", byName["memcached.abc123.cfg.use1.cache.amazonaws.com"]["engine"])
	assert.Equal(t, "valkey-serverless", byName["valkey-serverless-abc123.serverless.use1.cache.amazonaws.com"]["serverless_cache_name"])
}

func TestElastiCacheGetResourceReportsListingErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>AccessDenied</Code><Message>denied</Message></Error></ErrorResponse>`))
	}))
	t.Cleanup(server.Close)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(server.URL),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
		MaxRetries:  aws.Int(0),
	})
	require.NoError(t, err)

	provider := &elastiCacheProvider{
		options: ProviderOptions{Id: "test"},
		session: sess,
		regions: &ec2.DescribeRegionsOutput{Regions: []*ec2.Region{{RegionName: aws.String("us-east-1")}, {RegionName: aws.String("eu-west-1")}}},
	}
	_, err = provider.GetResource(context.Background())
	require.Error(t, err, "a denied listing in every region must not look like an empty account")
}
