package main

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"strings"
)

func getStackResources(ctx context.Context, config aws.Config, search *string) []cloudformation.StackResource {
	sc := *servicecatalog.New(config)
	cf := *cloudformation.New(config)

	var resources []cloudformation.StackResource
	for _, stack := range listStacks(ctx, cf, search) {
		for _, resource := range describeStackResources(ctx, cf, stack.StackName) {
			if "AWS::ServiceCatalog::CloudFormationProduct" == *resource.ResourceType {
				for _, product := range searchProvisionedProducts(ctx, sc, resource.PhysicalResourceId) {
					resources = append(resources, getStackResources(ctx, config, product.Id)...)
				}
			} else {
				resources = append(resources, resource)
			}
		}
	}
	return resources
}

func searchProvisionedProducts(ctx context.Context, client servicecatalog.Client, id *string) []servicecatalog.ProvisionedProductAttribute {
	var provisionedProducts []servicecatalog.ProvisionedProductAttribute
	var accessLevelFilterValueSelf = "self"
	searchQuery, err := servicecatalog.ProvisionedProductViewFilterBySearchQuery.MarshalValue()
	if err != nil {
		panic(err.Error())
	}

	var token *string
	for {
		input := &servicecatalog.SearchProvisionedProductsInput{
			PageToken: token,
			AccessLevelFilter: &servicecatalog.AccessLevelFilter{
				Key:   servicecatalog.AccessLevelFilterKeyAccount,
				Value: &accessLevelFilterValueSelf,
			},
			Filters: map[string][]string{
				searchQuery: {*id},
			},
		}
		request := client.SearchProvisionedProductsRequest(input)
		response, err := request.Send(ctx)
		if err != nil {
			panic(err.Error())
		}

		token = response.NextPageToken
		provisionedProducts = append(provisionedProducts, response.ProvisionedProducts...)

		if token == nil {
			break
		}
	}

	return provisionedProducts
}

func describeStackResources(ctx context.Context, client cloudformation.Client, stackName *string) []cloudformation.StackResource {
	input := &cloudformation.DescribeStackResourcesInput{
		StackName: stackName,
	}
	request := client.DescribeStackResourcesRequest(input)
	response, err := request.Send(ctx)
	if err != nil {
		panic(err.Error())
	}
	return response.StackResources
}

func listStacks(ctx context.Context, client cloudformation.Client, search *string) []cloudformation.StackSummary {
	var stacks []cloudformation.StackSummary
	var token *string
	for {
		input := &cloudformation.ListStacksInput{
			NextToken: token,
			StackStatusFilter: []cloudformation.StackStatus{
				cloudformation.StackStatusCreateComplete,
				cloudformation.StackStatusUpdateComplete,
			},
		}

		request := client.ListStacksRequest(input)
		response, err := request.Send(ctx)
		if err != nil {
			panic(err.Error())
		}

		token = response.NextToken
		for _, s := range response.StackSummaries {
			if strings.Contains(*s.StackName, *search) {
				stacks = append(stacks, s)
			}
		}

		if token == nil {
			break
		}
	}
	return stacks
}

func getAccount(ctx context.Context, config aws.Config) string {
	client := sts.New(config)
	response, err := client.GetCallerIdentityRequest(&sts.GetCallerIdentityInput{}).Send(ctx)
	if err != nil {
		panic(err)
	}
	return *response.Account
}