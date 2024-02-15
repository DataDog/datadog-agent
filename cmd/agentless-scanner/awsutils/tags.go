package awsutils

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func cloudResourceTagSpec(scan *types.ScanTask, target types.CloudID, resourceType types.ResourceType) []ec2types.TagSpecification {
	var resType ec2types.ResourceType
	switch resourceType {
	case types.ResourceTypeVolume:
		resType = ec2types.ResourceTypeVolume
	case types.ResourceTypeSnapshot:
		resType = ec2types.ResourceTypeSnapshot
	default:
		panic(fmt.Errorf("unexpected resource type %q", resourceType))
	}
	return []ec2types.TagSpecification{
		{
			ResourceType: resType,
			Tags: []ec2types.Tag{
				{Key: aws.String("DatadogAgentlessScanner"), Value: aws.String("true")},
				{Key: aws.String("DatadogAgentlessScannerTarget"), Value: aws.String(target.AsText())},
				{Key: aws.String("DatadogAgentlessScannerScanId"), Value: aws.String(scan.ID)},
				{Key: aws.String("DatadogAgentlessScannerScannerHostname"), Value: aws.String(scan.ScannerID.Hostname)},
			},
		},
	}
}

func cloudResourceTagFilters() []ec2types.Filter {
	return []ec2types.Filter{
		{
			Name: aws.String("tag:DatadogAgentlessScanner"),
			Values: []string{
				"true",
			},
		},
	}
}
