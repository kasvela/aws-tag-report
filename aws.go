package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/configservice"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"strings"
)

var ignoreErrors = map[string]struct{}{
	configservice.ErrCodeResourceNotFoundException: {},
	glue.ErrCodeEntityNotFoundException:            {},
	ErrCodeTagsNotSupportedException:               {},
}

type ResourceType string

var ResourceTypeCloudFormationProduct ResourceType = "AWS::ServiceCatalog::CloudFormationProduct"
var ResourceTypeCustomResource ResourceType = "Custom::"

const ErrCodeTagsNotSupportedException = "TagsNotSupportedException"

type TagsNotSupportedError struct {
	msg string
}
func (e *TagsNotSupportedError) Error() string {
	return fmt.Sprintln(e.Code(), e.Message())
}
func (e *TagsNotSupportedError) Code() string {
	return fmt.Sprint(ErrCodeTagsNotSupportedException)
}
func (e *TagsNotSupportedError) Message() string {
	return fmt.Sprint(e.msg, " does not support tagging")
}

func getStackResources(ctx context.Context, config aws.Config, search string) []cloudformation.StackResource {
	sc := *servicecatalog.New(config)
	cf := *cloudformation.New(config)

	var resources []cloudformation.StackResource
	for _, stack := range listStacks(ctx, cf, search) {
		for _, resource := range describeStackResources(ctx, cf, *stack.StackName) {
			if string(ResourceTypeCloudFormationProduct) == *resource.ResourceType {
				for _, product := range searchProvisionedProducts(ctx, sc, *resource.PhysicalResourceId) {
					resources = append(resources, getStackResources(ctx, config, *product.Id)...)
				}
			} else {
				resources = append(resources, resource)
			}
		}
	}
	return resources
}

func searchProvisionedProducts(ctx context.Context, client servicecatalog.Client, id string) []servicecatalog.ProvisionedProductAttribute {
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
				searchQuery: {id},
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

func describeStackResources(ctx context.Context, client cloudformation.Client, stackName string) []cloudformation.StackResource {
	input := &cloudformation.DescribeStackResourcesInput{
		StackName: &stackName,
	}
	request := client.DescribeStackResourcesRequest(input)
	response, err := request.Send(ctx)
	if err != nil {
		panic(err.Error())
	}
	return response.StackResources
}

func listStacks(ctx context.Context, client cloudformation.Client, search string) []cloudformation.StackSummary {
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
			if strings.Contains(*s.StackName, search) {
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

func ignoreError(err error) bool {
	var ae awserr.Error
	if ok := errors.As(err, &ae); ok {
		_, ignored := ignoreErrors[ae.Code()]
		return ignored
	}
	return false
}

func customResource(resourceType string) bool {
	return strings.HasPrefix(resourceType, string(ResourceTypeCustomResource))
}