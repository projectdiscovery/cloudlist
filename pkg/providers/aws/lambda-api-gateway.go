package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// apiGatewayProvider is a provider for AWS API Gateway resources
type lambdaAndapiGatewayProvider struct {
	id           string
	lambdaClient *lambda.Lambda
	apiGateway   *apigateway.APIGateway
	session      *session.Session
	regions      *ec2.DescribeRegionsOutput
}

// GetResource returns all the resources in the store for a provider.
func (ap *lambdaAndapiGatewayProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, region := range ap.regions.Regions {
		regionName := *region.RegionName
		sess, err := session.NewSession(&aws.Config{
			// Endpoint: aws.String("http://localhost:4566"),
			Region: aws.String(regionName)},
		)
		if err != nil {
			return nil, errors.Wrapf(err, "could not create session for region %s", regionName)
		}

		ap.apiGateway = apigateway.New(sess)
		ap.lambdaClient = lambda.New(sess)

		err = listAPIGatewayResources(ap.apiGateway, list, sess, ap.lambdaClient)
		if err != nil {
			return nil, errors.Wrapf(err, "could not list API Gateway resources for region %s", regionName)
		}
	}
	return list, nil
}

func listAPIGatewayResources(apiGateway *apigateway.APIGateway, list *schema.Resources, sess *session.Session, lambdaClient *lambda.Lambda) error {
	apis, err := apiGateway.GetRestApis(nil)
	if err != nil {
		return errors.Wrap(err, "could not list APIs")
	}
	// List Lambda functions and create a mapping of function ARN to function name
	lambdaFunctions, err := lambdaClient.ListFunctions(nil)
	if err != nil {
		return errors.Wrap(err, "could not list Lambda functions")
	}
	lambdaFunctionMapping := make(map[string]string)
	for _, lambdaFunction := range lambdaFunctions.Functions {
		lambdaFunctionMapping[*lambdaFunction.FunctionArn] = *lambdaFunction.FunctionName
	}

	// Iterate over each API Gateway resource
	for _, api := range apis.Items {
		// Construct base URL
		apiBaseURL := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", *api.Id, *sess.Config.Region)
		// Get resources for the API
		resources, err := apiGateway.GetResources(&apigateway.GetResourcesInput{
			RestApiId: api.Id,
		})
		if err != nil {
			return errors.Wrapf(err, "could not get resources for API %s", *api.Id)
		}
		list.Append(&schema.Resource{
			Provider: "aws",
			ID:       *api.Id,
			DNSName:  apiBaseURL,
			Public:   true,
		})

		for _, resource := range resources.Items {
			// List methods for the resource
			for _, method := range resource.ResourceMethods {
				integration, err := apiGateway.GetIntegration(&apigateway.GetIntegrationInput{
					RestApiId:  api.Id,
					ResourceId: resource.Id,
					HttpMethod: aws.String(*method.HttpMethod),
				})
				if err != nil {
					continue
				}
				// Check if the integration type is AWS_PROXY (indicating Lambda integration)
				if integration.Type != nil && *integration.Type == "AWS_PROXY" {
					functionARN := extractLambdaARN(*integration.Uri)
					functionName := lambdaFunctionMapping[functionARN]
					if functionName != "" {
						apiURLWithLambda := fmt.Sprintf("%s/lambda/%s", apiBaseURL, functionName)
						list.Append(&schema.Resource{
							Provider: "aws",
							ID:       *api.Id,
							DNSName:  apiURLWithLambda,
							Public:   true,
						})
					}
				}
			}
		}
	}
	return nil
}

// extract Lambda function ARN from integration URI
// Example URI: "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:123456789012:function:my-function/invocations"
func extractLambdaARN(uri string) string {
	parts := strings.Split(uri, "/")
	if len(parts) >= 5 {
		return parts[3]
	}
	return ""
}
