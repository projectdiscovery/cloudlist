package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// rdsProvider is a provider for AWS RDS API
type rdsProvider struct {
	options   ProviderOptions
	rdsClient *rds.RDS
	session   *session.Session
	regions   *ec2.DescribeRegionsOutput
}

func (rp *rdsProvider) name() string {
	return "rds"
}

// GetResource returns all the resources in the store for a provider.
func (rp *rdsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, region := range rp.regions.Regions {
		for _, rdsClient := range rp.getRdsClients(region.RegionName) {
			wg.Add(1)

			go func(client *rds.RDS) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						mu.Lock()
						errs = append(errs, fmt.Errorf("panic in rds provider: %v", r))
						mu.Unlock()
					}
				}()

				resources, err := rp.listRDSResources(ctx, client)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, err)
					return
				}
				list.Merge(resources)
			}(rdsClient)
		}
	}
	wg.Wait()
	if len(errs) > 0 && len(list.Items) == 0 {
		return nil, fmt.Errorf("rds: all workers failed: %v", errs)
	}
	return list, nil
}

func (rp *rdsProvider) listRDSResources(ctx context.Context, rdsClient *rds.RDS) (*schema.Resources, error) {
	list := schema.NewResources()

	err := rdsClient.DescribeDBInstancesPagesWithContext(ctx, &rds.DescribeDBInstancesInput{}, func(page *rds.DescribeDBInstancesOutput, _ bool) bool {
		for _, instance := range page.DBInstances {
			if instance == nil || instance.Endpoint == nil || aws.StringValue(instance.Endpoint.Address) == "" {
				continue
			}

			var metadata map[string]string
			if rp.options.ExtendedMetadata {
				metadata = getDBInstanceMetadata(instance)
			}

			list.Append(&schema.Resource{
				ID:       rp.options.Id,
				Provider: providerName,
				DNSName:  aws.StringValue(instance.Endpoint.Address),
				Public:   true,
				Service:  rp.name(),
				Metadata: metadata,
			})
		}
		return true
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not describe RDS instances")
	}

	err = rdsClient.DescribeDBClustersPagesWithContext(ctx, &rds.DescribeDBClustersInput{}, func(page *rds.DescribeDBClustersOutput, _ bool) bool {
		for _, cluster := range page.DBClusters {
			if cluster == nil {
				continue
			}

			var metadata map[string]string
			if rp.options.ExtendedMetadata {
				metadata = getDBClusterMetadata(cluster)
			}

			endpoints := append([]*string{cluster.Endpoint, cluster.ReaderEndpoint}, cluster.CustomEndpoints...)
			for _, endpoint := range endpoints {
				if aws.StringValue(endpoint) == "" {
					continue
				}
				list.Append(&schema.Resource{
					ID:       rp.options.Id,
					Provider: providerName,
					DNSName:  aws.StringValue(endpoint),
					Public:   true,
					Service:  rp.name(),
					Metadata: metadata,
				})
			}
		}
		return true
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not describe RDS clusters")
	}
	return list, nil
}

func getDBInstanceMetadata(instance *rds.DBInstance) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "db_instance_identifier", instance.DBInstanceIdentifier)
	schema.AddMetadata(metadata, "db_instance_arn", instance.DBInstanceArn)
	schema.AddMetadata(metadata, "db_instance_class", instance.DBInstanceClass)
	schema.AddMetadata(metadata, "db_instance_status", instance.DBInstanceStatus)
	schema.AddMetadata(metadata, "db_cluster_identifier", instance.DBClusterIdentifier)
	schema.AddMetadata(metadata, "engine", instance.Engine)
	schema.AddMetadata(metadata, "engine_version", instance.EngineVersion)
	schema.AddMetadata(metadata, "availability_zone", instance.AvailabilityZone)
	if instance.Endpoint != nil && instance.Endpoint.Port != nil {
		metadata["port"] = strconv.FormatInt(aws.Int64Value(instance.Endpoint.Port), 10)
	}
	if instance.DBSubnetGroup != nil {
		schema.AddMetadata(metadata, "vpc_id", instance.DBSubnetGroup.VpcId)
	}
	// RDS DNS names are always emitted; this tells reachable endpoints apart from VPC-only ones.
	metadata["publicly_accessible"] = strconv.FormatBool(aws.BoolValue(instance.PubliclyAccessible))
	if instance.InstanceCreateTime != nil {
		metadata["create_time"] = instance.InstanceCreateTime.Format(time.RFC3339)
	}
	if tagString := buildRDSTagString(instance.TagList); tagString != "" {
		metadata["tags"] = tagString
	}
	return metadata
}

func getDBClusterMetadata(cluster *rds.DBCluster) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "db_cluster_identifier", cluster.DBClusterIdentifier)
	schema.AddMetadata(metadata, "db_cluster_arn", cluster.DBClusterArn)
	schema.AddMetadata(metadata, "db_cluster_status", cluster.Status)
	schema.AddMetadata(metadata, "engine", cluster.Engine)
	schema.AddMetadata(metadata, "engine_version", cluster.EngineVersion)
	schema.AddMetadata(metadata, "engine_mode", cluster.EngineMode)
	if cluster.Port != nil {
		metadata["port"] = strconv.FormatInt(aws.Int64Value(cluster.Port), 10)
	}
	// Only set for Multi-AZ DB clusters; Aurora tracks it per member instance.
	if cluster.PubliclyAccessible != nil {
		metadata["publicly_accessible"] = strconv.FormatBool(aws.BoolValue(cluster.PubliclyAccessible))
	}
	if cluster.ClusterCreateTime != nil {
		metadata["create_time"] = cluster.ClusterCreateTime.Format(time.RFC3339)
	}
	if tagString := buildRDSTagString(cluster.TagList); tagString != "" {
		metadata["tags"] = tagString
	}
	return metadata
}

func buildRDSTagString(tags []*rds.Tag) string {
	var tagPairs []string
	for _, tag := range tags {
		if tag != nil && tag.Key != nil && tag.Value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
		}
	}
	return strings.Join(tagPairs, ",")
}

func (rp *rdsProvider) getRdsClients(region *string) []*rds.RDS {
	rdsClients := make([]*rds.RDS, 0)

	rdsClient := rds.New(
		rp.session,
		aws.NewConfig().WithRegion(aws.StringValue(region)),
	)
	rdsClients = append(rdsClients, rdsClient)

	if rp.options.AssumeRoleName == "" || len(rp.options.AccountIds) < 1 {
		return rdsClients
	}

	for _, accountId := range rp.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, rp.options.AssumeRoleName)
		creds := stscreds.NewCredentials(rp.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      region,
			Credentials: creds,
		})
		if err != nil {
			continue
		}

		rdsClients = append(rdsClients, rds.New(assumeSession))
	}
	return rdsClients
}
