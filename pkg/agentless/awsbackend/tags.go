// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package awsbackend

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func cloudResourceIsTagged(tags []ec2types.Tag) bool {
	for _, tag := range tags {
		if *tag.Key == "DatadogAgentlessScanner" && *tag.Value == "true" {
			return true
		}
	}
	return false

}

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
