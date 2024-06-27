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
		ap.apiGateway = apigateway.New(ap.session, aws.NewConfig().WithRegion(regionName))
		ap.lambdaClient = lambda.New(ap.session, aws.NewConfig().WithRegion(regionName))
		if resources, err := ap.listAPIGatewayResources(ap.apiGateway, regionName, ap.lambdaClient); err == nil {
			list.Merge(resources)
		}
	}
	return list, nil
}

func (ap *lambdaAndapiGatewayProvider) listAPIGatewayResources(apiGateway *apigateway.APIGateway, regionName string, lambdaClient *lambda.Lambda) (*schema.Resources, error) {
	list := schema.NewResources()
	apis, err := apiGateway.GetRestApis(&apigateway.GetRestApisInput{Limit: aws.Int64(500)})
	if err != nil {
		return nil, errors.Wrap(err, "could not list APIs")
	}
	// List Lambda functions and create a mapping of function ARN to function name
	lambdaFunctions, err := ap.getLambdaFunctions(lambdaClient)
	if err != nil {
		return nil, errors.Wrap(err, "could not list Lambda functions")
	}
	lambdaFunctionMapping := make(map[string]string)
	for _, lambdaFunction := range lambdaFunctions {
		lambdaFunctionMapping[*lambdaFunction.FunctionArn] = *lambdaFunction.FunctionName
	}
	// Iterate over each API Gateway resource
	for _, api := range apis.Items {
		apiBaseURL := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", *api.Id, regionName)
		list.Append(&schema.Resource{
			Provider: "aws",
			ID:       *api.Id,
			DNSName:  apiBaseURL,
			Public:   true,
			Service:  "apigateway",
		})
		// Get resources for the API
		resourceReq := &apigateway.GetResourcesInput{
			RestApiId: api.Id,
			Limit:     aws.Int64(100),
		}
		for {
			resources, err := apiGateway.GetResources(resourceReq)
			if err != nil {
				return nil, errors.Wrapf(err, "could not get resources for API %s", *api.Id)
			}

			for _, resource := range resources.Items {
				// List methods for the resource
				for _, method := range resource.ResourceMethods {
					if method == nil || method.HttpMethod == nil {
						continue
					}
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
						if functionName, ok := lambdaFunctionMapping[functionARN]; ok {
							apiURLWithLambda := fmt.Sprintf("%s/lambda/%s", apiBaseURL, functionName)
							list.Append(&schema.Resource{
								Provider: "aws",
								ID:       *api.Id,
								DNSName:  apiURLWithLambda,
								Public:   true,
								Service:  "lambda",
							})
						}
					}
				}
			}

			if aws.StringValue(resources.Position) == "" {
				break
			}
			resourceReq.SetPosition(*resources.Position)
		}
	}
	return list, nil
}

func (ap *lambdaAndapiGatewayProvider) getLambdaFunctions(lambdaClient *lambda.Lambda) ([]*lambda.FunctionConfiguration, error) {
	var lambdaFunctions []*lambda.FunctionConfiguration
	lambdaReq := &lambda.ListFunctionsInput{MaxItems: aws.Int64(20)}
	for {
		lambdaFuncs, err := lambdaClient.ListFunctions(lambdaReq)
		if err != nil {
			return nil, errors.Wrap(err, "could not list Lambda functions")
		}
		lambdaFunctions = append(lambdaFunctions, lambdaFuncs.Functions...)
		if aws.StringValue(lambdaFuncs.NextMarker) == "" {
			break
		}
		lambdaReq.SetMarker(*lambdaFuncs.NextMarker)
	}
	return lambdaFunctions, nil
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
