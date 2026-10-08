package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const describeDBInstancesPage1 = `<DescribeDBInstancesResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <DescribeDBInstancesResult>
    <Marker>page2</Marker>
    <DBInstances>
      <DBInstance>
        <DBInstanceIdentifier>public-db</DBInstanceIdentifier>
        <Engine>postgres</Engine>
        <PubliclyAccessible>true</PubliclyAccessible>
        <Endpoint>
          <Address>public-db.abc123.us-east-1.rds.amazonaws.com</Address>
          <Port>5432</Port>
        </Endpoint>
        <TagList><Tag><Key>env</Key><Value>prod</Value></Tag></TagList>
      </DBInstance>
      <DBInstance>
        <DBInstanceIdentifier>creating-db</DBInstanceIdentifier>
      </DBInstance>
    </DBInstances>
  </DescribeDBInstancesResult>
</DescribeDBInstancesResponse>`

const describeDBInstancesPage2 = `<DescribeDBInstancesResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <DescribeDBInstancesResult>
    <DBInstances>
      <DBInstance>
        <DBInstanceIdentifier>private-db</DBInstanceIdentifier>
        <Engine>mysql</Engine>
        <PubliclyAccessible>false</PubliclyAccessible>
        <Endpoint>
          <Address>private-db.abc123.us-east-1.rds.amazonaws.com</Address>
          <Port>3306</Port>
        </Endpoint>
      </DBInstance>
    </DBInstances>
  </DescribeDBInstancesResult>
</DescribeDBInstancesResponse>`

const describeDBClustersResponse = `<DescribeDBClustersResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <DescribeDBClustersResult>
    <DBClusters>
      <DBCluster>
        <DBClusterIdentifier>aurora-cluster</DBClusterIdentifier>
        <Engine>aurora-postgresql</Engine>
        <Port>5432</Port>
        <Endpoint>aurora-cluster.cluster-abc123.us-east-1.rds.amazonaws.com</Endpoint>
        <ReaderEndpoint>aurora-cluster.cluster-ro-abc123.us-east-1.rds.amazonaws.com</ReaderEndpoint>
        <CustomEndpoints>
          <member>analytics.cluster-custom-abc123.us-east-1.rds.amazonaws.com</member>
        </CustomEndpoints>
      </DBCluster>
    </DBClusters>
  </DescribeDBClustersResult>
</DescribeDBClustersResponse>`

func newTestRDSClient(t *testing.T) *rds.RDS {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "text/xml")
		switch r.Form.Get("Action") {
		case "DescribeDBInstances":
			if r.Form.Get("Marker") == "page2" {
				_, _ = w.Write([]byte(describeDBInstancesPage2))
				return
			}
			_, _ = w.Write([]byte(describeDBInstancesPage1))
		case "DescribeDBClusters":
			_, _ = w.Write([]byte(describeDBClustersResponse))
		default:
			t.Errorf("unexpected action %q", r.Form.Get("Action"))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(server.URL),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
	})
	require.NoError(t, err)
	return rds.New(sess)
}

func TestListRDSResources(t *testing.T) {
	provider := &rdsProvider{options: ProviderOptions{Id: "test", ExtendedMetadata: true}}

	resources, err := provider.listRDSResources(context.Background(), newTestRDSClient(t))
	require.NoError(t, err)

	byDNS := make(map[string]map[string]string)
	for _, item := range resources.Items {
		assert.Equal(t, "rds", item.Service)
		byDNS[item.DNSName] = item.Metadata
	}
	require.Len(t, byDNS, 5, "instances without an endpoint must be skipped, pagination must be followed")

	public := byDNS["public-db.abc123.us-east-1.rds.amazonaws.com"]
	require.NotNil(t, public)
	assert.Equal(t, "true", public["publicly_accessible"])
	assert.Equal(t, "5432", public["port"])
	assert.Equal(t, "env=prod", public["tags"])

	private := byDNS["private-db.abc123.us-east-1.rds.amazonaws.com"]
	require.NotNil(t, private)
	assert.Equal(t, "false", private["publicly_accessible"])

	for _, endpoint := range []string{
		"aurora-cluster.cluster-abc123.us-east-1.rds.amazonaws.com",
		"aurora-cluster.cluster-ro-abc123.us-east-1.rds.amazonaws.com",
		"analytics.cluster-custom-abc123.us-east-1.rds.amazonaws.com",
	} {
		require.Contains(t, byDNS, endpoint)
		assert.Equal(t, "aurora-cluster", byDNS[endpoint]["db_cluster_identifier"])
	}
}

func TestRDSGetResourceReportsListingErrors(t *testing.T) {
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

	provider := &rdsProvider{
		options: ProviderOptions{Id: "test"},
		session: sess,
		regions: &ec2.DescribeRegionsOutput{Regions: []*ec2.Region{{RegionName: aws.String("us-east-1")}, {RegionName: aws.String("eu-west-1")}}},
	}
	_, err = provider.GetResource(context.Background())
	require.Error(t, err, "a denied listing in every region must not look like an empty account")
}
