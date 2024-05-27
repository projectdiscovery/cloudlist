package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// Provider is a data provider for aws API
type Provider struct {
	id               string
	ec2Client        *ec2.EC2
	route53Client    *route53.Route53
	s3Client         *s3.S3
	ecsClient        *ecs.ECS
	lambdaClient     *lambda.Lambda
	apiGateway       *apigateway.APIGateway
	albClient        *elbv2.ELBV2
	elbClient        *elb.ELB
	lightsailClient  *lightsail.Lightsail
	cloudFrontClient *cloudfront.CloudFront
	regions          *ec2.DescribeRegionsOutput
	session          *session.Session
}

// New creates a new provider client for aws API
func New(options schema.OptionBlock) (*Provider, error) {
	accessKey, ok := options.GetMetadata(apiAccessKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	accessToken, ok := options.GetMetadata(apiSecretKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiSecretKey}
	}

	token, _ := options.GetMetadata(sessionToken)
	id, _ := options.GetMetadata("id")
	config := aws.NewConfig()
	config.WithRegion("us-east-1")
	config.WithCredentials(credentials.NewStaticCredentials(accessKey, accessToken, token))

	session, err := session.NewSession(config)
	if err != nil {
		return nil, errors.Wrap(err, "could not extablish a session")
	}

	ec2Client := ec2.New(session)
	route53Client := route53.New(session)
	s3Client := s3.New(session)
	ecsClient := ecs.New(session)
	lambdaClient := lambda.New(session)
	apiGateway := apigateway.New(session)
	albClient := elbv2.New(session)
	elbClient := elb.New(session)
	lightsailClient := lightsail.New(session)
	cloudFrontClient := cloudfront.New(session)

	regions, err := ec2Client.DescribeRegions(&ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, errors.Wrap(err, "could not get list of regions")
	}
	return &Provider{ec2Client: ec2Client, id: id, regions: regions, route53Client: route53Client, s3Client: s3Client, ecsClient: ecsClient, apiGateway: apiGateway, lambdaClient: lambdaClient, albClient: albClient, elbClient: elbClient, lightsailClient: lightsailClient, cloudFrontClient: cloudFrontClient, session: session}, nil
}

const apiAccessKey = "aws_access_key"
const apiSecretKey = "aws_secret_key"
const sessionToken = "aws_session_token"
const providerName = "aws"

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	ec2provider := &instanceProvider{ec2Client: p.ec2Client, id: p.id, session: p.session, regions: p.regions}
	list, err := ec2provider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	route53Provider := &route53Provider{route53: p.route53Client, id: p.id, session: p.session}
	zones, err := route53Provider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	s3Provider := &s3Provider{s3: p.s3Client, id: p.id, session: p.session}
	buckets, err := s3Provider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	ecsProvider := &ecsProvider{ecsClient: p.ecsClient, id: p.id, session: p.session, regions: p.regions}
	ecs, err := ecsProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	lamdaAndApiGatewayProvider := &lambdaAndapiGatewayProvider{apiGateway: p.apiGateway, lambdaClient: p.lambdaClient, id: p.id, session: p.session, regions: p.regions}
	lambdaAndApiGateways, err := lamdaAndApiGatewayProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	albProvider := &elbV2Provider{albClient: p.albClient, id: p.id, session: p.session, regions: p.regions}
	albs, err := albProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	elbProvider := &elbProvider{elbClient: p.elbClient, id: p.id, session: p.session, regions: p.regions}
	elbs, err := elbProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}

	lsRegions, err := p.lightsailClient.GetRegions(&lightsail.GetRegionsInput{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Lightsail regions")
	}
	lightsailProvider := &lightsailProvider{lsClient: p.lightsailClient, id: p.id, session: p.session, regions: lsRegions.Regions}
	lsInstances, err := lightsailProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	cloudfrontProvider := &cloudfrontProvider{cloudFrontClient: p.cloudFrontClient, id: p.id, session: p.session}
	cloudfrontResources, err := cloudfrontProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}

	finalList := schema.NewResources()
	finalList.Merge(list)
	finalList.Merge(lsInstances)
	finalList.Merge(zones)
	finalList.Merge(buckets)
	finalList.Merge(ecs)
	finalList.Merge(lambdaAndApiGateways)
	finalList.Merge(albs)
	finalList.Merge(elbs)
	finalList.Merge(cloudfrontResources)
	return finalList, nil
}
