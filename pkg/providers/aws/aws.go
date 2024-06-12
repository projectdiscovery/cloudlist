package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var supportedServices = []string{"ec2", "route53", "s3", "ecs", "eks", "lambda", "apigateway", "alb", "elb", "lightsail", "cloudfront"}

// Provider is a data provider for aws API
type Provider struct {
	id               string
	ec2Client        *ec2.EC2
	route53Client    *route53.Route53
	s3Client         *s3.S3
	ecsClient        *ecs.ECS
	eksClient        *eks.EKS
	lambdaClient     *lambda.Lambda
	apiGateway       *apigateway.APIGateway
	albClient        *elbv2.ELBV2
	elbClient        *elb.ELB
	lightsailClient  *lightsail.Lightsail
	cloudFrontClient *cloudfront.CloudFront
	regions          *ec2.DescribeRegionsOutput
	session          *session.Session
	services         schema.ServiceMap
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

	provider := &Provider{}
	token, _ := options.GetMetadata(sessionToken)
	provider.id, _ = options.GetMetadata("id")
	config := aws.NewConfig()
	config.WithRegion("us-east-1")
	config.WithCredentials(credentials.NewStaticCredentials(accessKey, accessToken, token))

	session, err := session.NewSession(config)
	if err != nil {
		return nil, errors.Wrap(err, "could not extablish a session")
	}
	provider.session = session
	rc := ec2.New(session)
	regions, err := rc.DescribeRegions(&ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, errors.Wrap(err, "could not get list of regions")
	}
	provider.regions = regions
	supportedServicesMap := make(map[string]struct{})
	for _, s := range supportedServices {
		supportedServicesMap[s] = struct{}{}
	}
	services := make(schema.ServiceMap)
	if ss, ok := options.GetMetadata("services"); ok {
		for _, s := range strings.Split(ss, ",") {
			if _, ok := supportedServicesMap[s]; ok {
				services[s] = struct{}{}
			}
		}
	}
	if len(services) == 0 {
		for _, s := range supportedServices {
			services[s] = struct{}{}
		}
	}
	provider.services = services

	if services.Has("ec2") {
		provider.ec2Client = ec2.New(session)
	}
	if services.Has("route53") {
		provider.route53Client = route53.New(session)
	}
	if services.Has("s3") {
		provider.s3Client = s3.New(session)
	}
	if services.Has("ecs") {
		provider.ecsClient = ecs.New(session)
	}
	if services.Has("eks") {
		provider.eksClient = eks.New(session)
	}
	if services.Has("lambda") {
		provider.lambdaClient = lambda.New(session)
	}
	if services.Has("apigateway") {
		provider.apiGateway = apigateway.New(session)
	}
	if services.Has("alb") {
		provider.albClient = elbv2.New(session)
	}
	if services.Has("elb") {
		provider.elbClient = elb.New(session)
	}
	if services.Has("lightsail") {
		provider.lightsailClient = lightsail.New(session)
	}
	if services.Has("cloudfront") {
		provider.cloudFrontClient = cloudfront.New(session)
	}
	return provider, nil
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

// Services returns the provider services
func (p *Provider) Services() []string {
	return p.services.Keys()
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalList := schema.NewResources()
	if p.ec2Client != nil {
		ec2provider := &instanceProvider{ec2Client: p.ec2Client, id: p.id, session: p.session, regions: p.regions}
		if list, err := ec2provider.GetResource(ctx); err == nil {
			finalList.Merge(list)
		}
	}
	if p.route53Client != nil {
		route53Provider := &route53Provider{route53: p.route53Client, id: p.id, session: p.session}
		if zones, err := route53Provider.GetResource(ctx); err == nil {
			finalList.Merge(zones)
		}
	}
	if p.s3Client != nil {
		s3Provider := &s3Provider{s3: p.s3Client, id: p.id, session: p.session}
		if buckets, err := s3Provider.GetResource(ctx); err == nil {
			finalList.Merge(buckets)
		}
	}
	if p.ecsClient != nil {
		ecsProvider := &ecsProvider{ecsClient: p.ecsClient, id: p.id, session: p.session, regions: p.regions}
		if ecsResources, err := ecsProvider.GetResource(ctx); err == nil {
			finalList.Merge(ecsResources)
		}
	}
	if p.eksClient != nil {
		eksProvider := &eksProvider{eksClient: p.eksClient, id: p.id, session: p.session, regions: p.regions}
		if eksResources, err := eksProvider.GetResource(ctx); err == nil {
			finalList.Merge(eksResources)
		}
	}
	if p.apiGateway != nil && p.lambdaClient != nil {
		lamdaAndApiGatewayProvider := &lambdaAndapiGatewayProvider{apiGateway: p.apiGateway, lambdaClient: p.lambdaClient, id: p.id, session: p.session, regions: p.regions}
		if lambdaAndApiGateways, err := lamdaAndApiGatewayProvider.GetResource(ctx); err == nil {
			finalList.Merge(lambdaAndApiGateways)
		}
	}
	if p.albClient != nil {
		albProvider := &elbV2Provider{albClient: p.albClient, id: p.id, session: p.session, regions: p.regions}
		if albs, err := albProvider.GetResource(ctx); err == nil {
			finalList.Merge(albs)

		}
	}
	if p.elbClient != nil {
		elbProvider := &elbProvider{elbClient: p.elbClient, id: p.id, session: p.session, regions: p.regions}
		if elbs, err := elbProvider.GetResource(ctx); err == nil {
			finalList.Merge(elbs)
		}
	}
	if p.lightsailClient != nil {
		lsRegions, err := p.lightsailClient.GetRegions(&lightsail.GetRegionsInput{})
		if err == nil {
			lightsailProvider := &lightsailProvider{lsClient: p.lightsailClient, id: p.id, session: p.session, regions: lsRegions.Regions}
			if lsInstances, err := lightsailProvider.GetResource(ctx); err == nil {
				finalList.Merge(lsInstances)
			}
		}
	}
	if p.cloudFrontClient != nil {
		cloudfrontProvider := &cloudfrontProvider{cloudFrontClient: p.cloudFrontClient, id: p.id, session: p.session}
		if cloudfrontResources, err := cloudfrontProvider.GetResource(ctx); err == nil {
			finalList.Merge(cloudfrontResources)
		}
	}
	return finalList, nil
}
