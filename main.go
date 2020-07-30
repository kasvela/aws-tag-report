package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchevents"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/configservice"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"os"
	"reflect"
	"strings"
)

var (
	resourceType = "resource-type"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: aws-tag-report searchString > reportFile" +
			"\n\tsearchString: will select any cloudformation stack with searchString within its name" +
			"\n\treportFile: file to redirect  csv output")
		return
	}

	ctx := context.TODO()
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	servicecatalogClient := servicecatalog.New(cfg)
	lambdaClient := lambda.New(cfg)
	ssmClient := ssm.New(cfg)
	s3Client := s3.New(cfg)
	glueClient := glue.New(cfg)
	iamClient := iam.New(cfg)
	snsClient := sns.New(cfg)
	ec2Client := ec2.New(cfg)
	dynamodbClient := dynamodb.New(cfg)
	firehoseClient := firehose.New(cfg)
	cloudwatchlogsClient := cloudwatchlogs.New(cfg)
	cloudwatchClient := cloudwatch.New(cfg)
	cloudwatcheventsClient := cloudwatchevents.New(cfg)
	configserviceClient := configservice.New(cfg)
	kmsClient := kms.New(cfg)

	region := cfg.Region
	account := getAccount(ctx, cfg)
	lookups := map[string]func(context.Context, aws.Config, string) (map[string]string, error) {
		// Lambda
		"AWS::Lambda::Function":
			wrap(lambdaClient.ListTagsRequest,
				InputParam{"Resource", arnF3(region, account, "lambda", "function")}),
		// SSM
		"AWS::SSM::Parameter":
			wrap(ssmClient.ListTagsForResourceRequest,
				InputParam{"ResourceId", physicalResourceId},
				InputParam{"ResourceType", ssm.ResourceTypeForTaggingParameter}),
		// Service Catalog
		"AWS::ServiceCatalog::CloudFormationProduct":
			wrap(servicecatalogClient.DescribeProductRequest,
				InputParam{"Id", physicalResourceId}),
		"AWS::ServiceCatalog::Portfolio":
			wrap(servicecatalogClient.DescribePortfolioRequest,
				InputParam{"Id", physicalResourceId}),
		// S3
		"AWS::S3::Bucket":
			wrap(s3Client.GetBucketTaggingRequest,
				InputParam{"Bucket", physicalResourceId}),
		// IAM
		"AWS::IAM::Role":
			wrap(iamClient.ListRoleTagsRequest,
				InputParam{"RoleName", physicalResourceId}),
		// SNS
		"AWS::SNS::Topic":
			wrap(snsClient.ListTagsForResourceRequest,
				InputParam{"ResourceArn", physicalResourceId}),
		// EC2
		"AWS::EC2::LaunchTemplate":
			wrap(ec2Client.DescribeTagsRequest,
				InputParam{"Filters",
					[]ec2.Filter{{Name: &resourceType, Values: []string{"launch-template"}},}}),
		"AWS::EC2::RouteTable":
		wrap(ec2Client.DescribeTagsRequest,
			InputParam{"Filters",
				[]ec2.Filter{{Name: &resourceType, Values: []string{"route-table"}},}}),
		"AWS::EC2::SecurityGroup":
			wrap(ec2Client.DescribeTagsRequest,
				InputParam{"Filters",
					[]ec2.Filter{{Name: &resourceType, Values: []string{"security-group"}},}}),
		"AWS::EC2::Subnet":
			wrap(ec2Client.DescribeTagsRequest,
				InputParam{"Filters",
					[]ec2.Filter{{Name: &resourceType, Values: []string{"subnet"}},}}),
		"AWS::EC2::VPC":
			wrap(ec2Client.DescribeTagsRequest,
				InputParam{"Filters",
					[]ec2.Filter{{Name: &resourceType, Values: []string{"vpc"}},}}),
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
		// DynamoDB
		"AWS::DynamoDB::Table":
			wrap(dynamodbClient.ListTagsOfResourceRequest,
				InputParam{"ResourceArn", arnF2(region, account, "dynamodb", "table")}),
		// Kinesis Firehose
		"AWS::KinesisFirehose::DeliveryStream":
			wrap(firehoseClient.ListTagsForDeliveryStreamRequest,
				InputParam{"DeliveryStreamName", physicalResourceId}),
		// Cloudwatch Logs
		"AWS::Logs::LogGroup":
			wrap(cloudwatchlogsClient.ListTagsLogGroupRequest,
				InputParam{"LogGroupName", physicalResourceId}),
		// Cloudwatch
		"AWS::Cloudwatch::Alarm":
			wrap(cloudwatchClient.ListTagsForResourceRequest,
				InputParam{"ResourceARN", arnF3(region, account, "cloudwatch", "alarm")}),
		// Events
		"AWS::Events::Rule":
			wrap(cloudwatcheventsClient.ListTagsForResourceRequest,
				InputParam{"ResourceARN", arnF2(region, account, "events", "rule")}),
		// Config
		"AWS::Config::ConfigRule":
			wrap(configserviceClient.ListTagsForResourceRequest,
				InputParam{"ResourceArn", arnF2(region, account, "config", "config-rule")}),
		// KMS
		"AWS::KMS::Key":
			wrap(kmsClient.ListResourceTagsRequest,
				InputParam{"KeyId", physicalResourceId}),

		//////// TAGS NOT SUPPORTED ////////
		// Lambda
		"AWS::Lambda::Permission": nop("AWS::Lambda::Permission"),
		// Service Catalog
		"AWS::ServiceCatalog::LaunchRoleConstraint":          nop("AWS::ServiceCatalog::LaunchRoleConstraint"),
		"AWS::ServiceCatalog::PortfolioPrincipalAssociation": nop("AWS::ServiceCatalog::PortfolioPrincipalAssociation"),
		"AWS::ServiceCatalog::PortfolioProductAssociation":   nop("AWS::ServiceCatalog::PortfolioProductAssociation"),
		"AWS::ServiceCatalog::TagOptionAssociation":          nop("AWS::ServiceCatalog::TagOptionAssociation"),
		"AWS::ServiceCatalog::TagOption":                     nop("AWS::ServiceCatalog::TagOption"),
		// S3
		"AWS::S3::BucketPolicy": nop("AWS::S3::BucketPolicy"),
		// IAM
		"AWS::IAM::InstanceProfile": nop("AWS::IAM::InstanceProfile"),
		"AWS::IAM::Policy":          nop("AWS::IAM::Policy"),
		// SNS
		"AWS::SNS::Subscription": nop("AWS::SNS::Subscription"),
		"AWS::SNS::TopicPolicy":  nop("AWS::SNS::TopicPolicy"),
		// EC2
		"AWS::EC2::VPCEndpoint":                 nop("AWS::EC2::VPCEndpoint"),
		"AWS::EC2::SubnetRouteTableAssociation": nop("AWS::EC2::SubnetRouteTableAssociation"),
		"AWS::EC2::SecurityGroupIngress":        nop("AWS::EC2::SecurityGroupIngress"),
		// Glue
		"AWS::Glue::Database":              nop("AWS::Glue::Database"),
		"AWS::Glue::SecurityConfiguration": nop("AWS::Glue::SecurityConfiguration"),
		// Batch
		"AWS::Batch::JobDefinition":      nop("AWS::Batch::JobDefinition"),
		"AWS::Batch::JobQueue":           nop("AWS::Batch::JobQueue"),
		"AWS::Batch::ComputeEnvironment": nop("AWS::Batch::ComputeEnvironment"),
		// Logs
		"AWS::Logs::LogStream": nop("AWS::Logs::LogStream"),
		// CloudFormation
		"AWS::CloudFormation::Macro": nop("AWS::CloudFormation::Macro"),
	}

	search := &os.Args[1]
	report := NewReporter()

	for r, resource := range getStackResources(ctx, cfg, search) {
		// custom resources do not support tags
		if strings.HasPrefix(*resource.ResourceType, "Custom::") {
			err := TagsNotSupportedError{*resource.ResourceType}
			fmt.Fprintln(os.Stderr, err.Error())
			report.AddNotSupported(*resource.ResourceType, *resource.PhysicalResourceId, *resource.StackName, *search)
			continue
		}
		// get the proper tag lookup function
		if lookup, ok := lookups[*resource.ResourceType]; ok {
			tags, err := lookup(ctx, cfg, *resource.PhysicalResourceId)
			if err == nil {
				// tags lookup succeeded
				report.Add(*resource.ResourceType, *resource.PhysicalResourceId, *resource.StackName, *search, tags)
			} else {
				// some errors should not stop processing resources
				var ae awserr.Error
				if ok := errors.As(err, &ae); ok && (
						ae.Code() == configservice.ErrCodeResourceNotFoundException ||
						ae.Code() == glue.ErrCodeEntityNotFoundException){
					fmt.Fprintln(os.Stderr, ae.Error())
					report.AddNotSupported(*resource.ResourceType, *resource.PhysicalResourceId, *resource.StackName, *search)
				} else if ne, ok := err.(*TagsNotSupportedError); ok {
					fmt.Fprintln(os.Stderr, ne.Error())
					report.AddNotSupported(*resource.ResourceType, *resource.PhysicalResourceId, *resource.StackName, *search)
				} else {
					fmt.Fprintln(os.Stderr, reflect.TypeOf(err), Prettify(resource))
					panic(err.Error())
				}
			}
		} else {
			err := NotImplementedError{*resource.ResourceType}
			fmt.Fprintln(os.Stderr, err.Error(), Prettify(resource))
			panic(err.Error())
		}

		if r % 1000 == 0 {
			report.Write()
		}
	}

	report.Write()
}
