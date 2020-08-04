package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchevents"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/configservice"
	"github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
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
)

var (
	resourceType = "resource-type"
)

func main() {
	if len(os.Args) <= 1 {
		fmt.Println("usage: aws-tag-report s1 [s2 ...] > report" +
			"\n  list resources for each matched stack and get tags for each res" +
			"\n  s1 s2 ...: substrings used to match cloudformation stack names" +
			"\n  report: file to redirect csv output")
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
	dmsClient := databasemigrationservice.New(cfg)

	region := cfg.Region
	account := getAccount(ctx, cfg)
	lookups := map[string]func(context.Context, aws.Config, string) (map[string]string, error){
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
		// DMS,
		"AWS::DMS::EventSubscription":
		wrap(dmsClient.ListTagsForResourceRequest,
			InputParam{"ResourceArn", arnF3(region, account, "dms", "es")}),
	}

	tagsNotSupported:= map[string]struct{}{
		// Lambda
		"AWS::Lambda::Permission": {},
		// Service Catalog
		"AWS::ServiceCatalog::LaunchRoleConstraint":          {},
		"AWS::ServiceCatalog::PortfolioPrincipalAssociation": {},
		"AWS::ServiceCatalog::PortfolioProductAssociation":   {},
		"AWS::ServiceCatalog::TagOptionAssociation":          {},
		"AWS::ServiceCatalog::TagOption":                     {},
		// S3
		"AWS::S3::BucketPolicy": {},
		// IAM
		"AWS::IAM::InstanceProfile": {},
		"AWS::IAM::Policy":          {},
		// SNS
		"AWS::SNS::Subscription": {},
		"AWS::SNS::TopicPolicy":  {},
		// EC2
		"AWS::EC2::VPCEndpoint":                 {},
		"AWS::EC2::SubnetRouteTableAssociation": {},
		"AWS::EC2::SecurityGroupIngress":        {},
		// Glue
		"AWS::Glue::Database":              {},
		"AWS::Glue::SecurityConfiguration": {},
		// Batch
		"AWS::Batch::JobDefinition":      {},
		"AWS::Batch::JobQueue":           {},
		"AWS::Batch::ComputeEnvironment": {},
		// Logs
		"AWS::Logs::LogStream": {},
		// CloudFormation
		"AWS::CloudFormation::Macro": {},
	}

	searchs := os.Args[1:]
	report := NewReporter()

	fmt.Print("\nResources supported: ")
	for k, _ := range lookups {
		fmt.Print(k, ",")
	}
	fmt.Print("\nResources not supporting tags: ")
	for k, _ := range tagsNotSupported {
		fmt.Print(k, ",")
	}

	for _, s := range searchs {
		for i, res := range getStackResources(ctx, cfg, s) {
			_, noSupport := tagsNotSupported[*res.ResourceType]
			if customResource(*res.ResourceType) || noSupport {
				fmt.Fprintln(os.Stderr, (&TagsNotSupportedError{*res.ResourceType}).Error())
				report.NotSupported(*res.ResourceType, *res.LogicalResourceId, *res.StackName, s)
				continue
			}

			if lookup, ok := lookups[*res.ResourceType]; ok {
				tags, err := lookup(ctx, cfg, *res.PhysicalResourceId)
				if err == nil {
					report.Add(*res.ResourceType, *res.PhysicalResourceId, *res.StackName, s, tags)
				} else {
					if ignoreError(err) {
						fmt.Fprintln(os.Stderr, err.Error())
						report.NotSupported(*res.ResourceType, *res.PhysicalResourceId, *res.StackName, s)
					} else {
						fmt.Fprintln(os.Stderr, reflect.TypeOf(err), Prettify(res))
						panic(err.Error())
					}
				}
			} else {
				err := NotImplementedError{*res.ResourceType}
				fmt.Fprintln(os.Stderr, err.Error(), Prettify(res))
				panic(err.Error())
			}

			if i% 1000 == 0 {
				report.Write()
			}
		}
	}

	report.Write()
}
