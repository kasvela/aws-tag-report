package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("USAGE: aws-tag-report searchString")
		return
	}

	ctx := context.TODO()
	config, err := external.LoadDefaultAWSConfig()
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	servicecatalogClient := servicecatalog.New(config)
	lambdaClient := lambda.New(config)
	ssmClient := ssm.New(config)
	s3Client := s3.New(config)
	glueClient := glue.New(config)
	iamClient := iam.New(config)

	region := config.Region
	account := getAccount(ctx, config)
	lookups := map[string]func(context.Context, aws.Config, string) (map[string]string, error) {
		// Lambda
		"AWS::Lambda::Function":
			wrap(lambdaClient.ListTagsRequest,
				InputParam{"Resource", arnF3(region, account, "lambda", "function")}),
		// SSM
		"AWS::SSM::Parameter":
			wrap(ssmClient.ListTagsForResourceRequest,
				InputParam{"ResourceId", resourceId},
				InputParam{"ResourceType", ssm.ResourceTypeForTaggingParameter}),
		// Service Catalog
		"AWS::ServiceCatalog::CloudFormationProduct":
			wrap(servicecatalogClient.DescribeProductRequest,
				InputParam{"Id", resourceId}),
		"AWS::ServiceCatalog::Portfolio":
			wrap(servicecatalogClient.DescribePortfolioRequest,
				InputParam{"Id", resourceId}),
		// S3
		"AWS::S3::Bucket":
			wrap(s3Client.GetBucketTaggingRequest,
				InputParam{"Bucket", resourceId}),
		// IAM
		"AWS::IAM::Role":
			wrap(iamClient.ListRoleTagsRequest,
				InputParam{"RoleName", resourceId}),

		// Glue
		"AWS::Glue::Crawler":
			wrap(glueClient.GetTagsRequest,
				InputParam{"ResourceArn", arnF2(region, account, "glue", "crawler")}),
		"AWS::Glue::Job":
			wrap(glueClient.GetTagsRequest,
				InputParam{"ResourceArn", arnF2(region, account, "glue", "job")}),
		"AWS::Glue::Trigger":
			wrap(glueClient.GetTagsRequest,
				InputParam{"ResourceArn", arnF2(region, account, "glue", "trigger")}),

		//////// TAGS NOT SUPPORTED ////////
		// Service Catalog
		"AWS::ServiceCatalog::LaunchRoleConstraint":          nop("AWS::ServiceCatalog::LaunchRoleConstraint"),
		"AWS::ServiceCatalog::PortfolioPrincipalAssociation": nop("AWS::ServiceCatalog::PortfolioPrincipalAssociation"),
		"AWS::ServiceCatalog::PortfolioProductAssociation":   nop("AWS::ServiceCatalog::PortfolioProductAssociation"),
		"AWS::ServiceCatalog::TagOptionAssociation":          nop("AWS::ServiceCatalog::TagOptionAssociation"),
		"AWS::ServiceCatalog::TagOption":                     nop("AWS::ServiceCatalog::TagOption"),
		// Glue
		"AWS::Glue::Database": nop("AWS::Glue::Database"),
	}

	search := &os.Args[1]
	for _, resource := range getStackResources(ctx, config, search) {
		if lookup, ok := lookups[*resource.ResourceType]; ok {
			tags, err := lookup(ctx, config, *resource.PhysicalResourceId)
			if err == nil {
				println(*resource.PhysicalResourceId, Prettify(tags))
			} else {
				if noTagsErr, ok := err.(*TagsNotSupportedError); ok {
					fmt.Fprintln(os.Stderr, noTagsErr.Error())
				} else {
					panic(err)
				}
			}
		} else {
			err := NotImplementedError{*resource.ResourceType}
			fmt.Fprintln(os.Stderr, err.Error())
			panic(err)
		}
	}
}
