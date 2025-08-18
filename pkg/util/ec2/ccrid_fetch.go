// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
)

var (
	getInstanceID = GetInstanceID
	getRegion     = GetRegion
	getAccountID  = GetAccountID
)

var regionFetcher = cachedfetch.Fetcher{
	Name: "EC2 Region",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return ec2internal.GetMetadataItemWithMaxLength(ctx, "/placement/region", ec2internal.UseIMDSv2(), false)
	},
}

// GetRegion returns the AWS region as reported by EC2 IMDS.
func GetRegion(ctx context.Context) (string, error) {
	return regionFetcher.FetchString(ctx)
}

// GetHostCCRID returns the EC2 instance ARN for use as host CCRID
func GetHostCCRID(ctx context.Context) (string, error) {
	instanceID, err := getInstanceID(ctx)
	if err != nil {
		return "", err
	}
	region, err := getRegion(ctx)
	if err != nil {
		return "", err
	}
	accountID, err := getAccountID(ctx)
	if err != nil {
		return "", err
	}

	arn := fmt.Sprintf("arn:aws:ec2:%s:%s:instance/%s", region, accountID, instanceID)
	return arn, nil
}
