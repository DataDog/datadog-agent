// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build ec2

package aws

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

type mockrdsServiceConfigurer func(k *MockrdsService)

const defaultDbmTag = "datadoghq.com/dbm:true"
const defaultGlobalViewDbTag = "datadoghq.com/global_view_db"

func createDescribeDBInstancesRequest(clusterIDs []string) *rds.DescribeDBInstancesInput {
	return &rds.DescribeDBInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("db-cluster-id"),
				Values: clusterIDs,
			},
		},
	}
}
