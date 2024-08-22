package aws

import (
	"context"
	"strings"
	"sync"

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
	sliceutil "github.com/projectdiscovery/utils/slice"
)

var Services = []string{"ec2", "instance", "route53", "s3", "ecs", "eks", "lambda", "apigateway", "alb", "elb", "lightsail", "cloudfront"}

type ProviderOptions struct {
	Id             string
	AccessKey      string
	SecretKey      string
	Token          string
	AssumeRoleName string
	AccountIds     []string
	Services       schema.ServiceMap
}

func (p *ProviderOptions) ParseOptionBlock(block schema.OptionBlock) error {
	accessKey, ok := block.GetMetadata(apiAccessKey)
	if !ok {
		return &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	accessToken, ok := block.GetMetadata(apiSecretKey)
	if !ok {
		return &schema.ErrNoSuchKey{Name: apiSecretKey}
	}
	p.Token, _ = block.GetMetadata(sessionToken)
	p.Id, _ = block.GetMetadata("id")

	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}
	services := make(schema.ServiceMap)
	if ss, ok := block.GetMetadata("services"); ok {
		for _, s := range strings.Split(ss, ",") {
			if _, ok := supportedServicesMap[s]; ok {
				services[s] = struct{}{}
			}
		}
	}
	// if no services provided from -service flag, includes all services
	if len(services) == 0 {
		for _, s := range Services {
			services[s] = struct{}{}
		}
	}

	p.AccessKey = accessKey
	p.SecretKey = accessToken
	p.Services = services

	if assumeRoleName, ok := block.GetMetadata(assumeRoleName); ok {
		p.AssumeRoleName = assumeRoleName
	}

	if accountIds, ok := block.GetMetadata(accountIds); ok {
		p.AccountIds = sliceutil.Dedupe(strings.Split(accountIds, ","))
	}
	return nil
}

// Provider is a data provider for aws API
type Provider struct {
	options          *ProviderOptions
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
}

// New creates a new provider client for aws API
func New(block schema.OptionBlock) (*Provider, error) {
	options := &ProviderOptions{}
	if err := options.ParseOptionBlock(block); err != nil {
		return nil, err
	}

	provider := &Provider{options: options}
	config := aws.NewConfig()
	config.WithRegion("us-east-1")
	config.WithCredentials(credentials.NewStaticCredentials(options.AccessKey, options.SecretKey, options.Token))

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

	services := provider.options.Services
	if services.Has("ec2") || services.Has("instance") {
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

const providerName = "aws"
const apiAccessKey = "aws_access_key"
const apiSecretKey = "aws_secret_key"
const sessionToken = "aws_session_token"
const assumeRoleName = "assume_role_name"
const accountIds = "account_ids"

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.options.Id
}

// Services returns the provider services
func (p *Provider) Services() []string {
	return p.options.Services.Keys()
}

type result struct {
	resources *schema.Resources
	err       error
}

type getResourcesFunc func(context.Context) (*schema.Resources, error)

func worker(ctx context.Context, fn getResourcesFunc, ch chan<- result) {
	resources, err := fn(ctx)
	ch <- result{resources, err}
}

func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()

	var workersWaitGroup sync.WaitGroup
	results := make(chan result)

	assignWorker := func(fn getResourcesFunc) {
		workersWaitGroup.Add(1)
		go func() {
			defer workersWaitGroup.Done()
			worker(ctx, fn, results)
		}()
	}

	if p.ec2Client != nil {
		ec2provider := &instanceProvider{ec2Client: p.ec2Client, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(ec2provider.GetResource)
	}
	if p.route53Client != nil {
		route53Provider := &route53Provider{route53: p.route53Client, options: *p.options, session: p.session}
		assignWorker(route53Provider.GetResource)
	}
	if p.s3Client != nil {
		s3Provider := &s3Provider{s3: p.s3Client, options: *p.options, session: p.session}
		assignWorker(s3Provider.GetResource)
	}
	if p.ecsClient != nil {
		ecsProvider := &ecsProvider{ecsClient: p.ecsClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(ecsProvider.GetResource)
	}
	if p.eksClient != nil {
		eksProvider := &eksProvider{eksClient: p.eksClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(eksProvider.GetResource)
	}
	if p.apiGateway != nil && p.lambdaClient != nil {
		lamdaAndApiGatewayProvider := &lambdaAndapiGatewayProvider{apiGateway: p.apiGateway, lambdaClient: p.lambdaClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(lamdaAndApiGatewayProvider.GetResource)
	}
	if p.albClient != nil {
		albProvider := &elbV2Provider{albClient: p.albClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(albProvider.GetResource)
	}
	if p.elbClient != nil {
		elbProvider := &elbProvider{elbClient: p.elbClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(elbProvider.GetResource)
	}
	if p.lightsailClient != nil {
		lsRegions, err := p.lightsailClient.GetRegions(&lightsail.GetRegionsInput{})
		if err == nil {
			lightsailProvider := &lightsailProvider{lsClient: p.lightsailClient, options: *p.options, session: p.session, regions: lsRegions.Regions}
			assignWorker(lightsailProvider.GetResource)
		}
	}
	if p.cloudFrontClient != nil {
		cloudfrontProvider := &cloudfrontProvider{cloudFrontClient: p.cloudFrontClient, options: *p.options, session: p.session}
		assignWorker(cloudfrontProvider.GetResource)
	}

	go func() {
		workersWaitGroup.Wait()
		close(results)
	}()

	for result := range results {
		if result.err != nil {
			continue
		}
		finalResources.Merge(result.resources)
	}
	return finalResources, nil
}
