// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -package=$GOPACKAGE -destination=rdsclient_mockgen.go -build_constraint "ec2 && test"

// RdsClient is the interface for describing cluster and instance endpoints
type RdsClient interface {
	GetAuroraClusterEndpoints(ctx context.Context, clusters []DBCluster, config Config) ([]Instance, error)
	GetAuroraClustersFromTags(ctx context.Context, tags []string) ([]DBCluster, error)
	GetRdsInstancesFromTags(ctx context.Context, config Config) ([]Instance, error)
}

// rdsService defines the interface for describing cluster instances. It exists here to facilitate testing
// while *rdsQueryClient is the implementation for production code.
type rdsService interface {
	DescribeDBInstances(ctx context.Context, params *DescribeDBInstancesInput) (*DescribeDBInstancesOutput, error)
	DescribeDBClusters(ctx context.Context, params *DescribeDBClustersInput) (*DescribeDBClustersOutput, error)
}

// Client is a wrapper around the AWS RDS client
type Client struct {
	client rdsService
}

// NewRdsClient creates a new AWS client for querying RDS
func NewRdsClient(region string) (*Client, string, error) {
	if region == "" {
		identity, err := ec2.GetInstanceIdentity(context.Background())
		if err != nil {
			return nil, "", err
		}
		region = identity.Region
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Try to load shared AWS configuration.
	// The default configuration sources are:
	// * Environment Variables
	// * Shared Configuration and Shared Credentials files.
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, region, err
	}

	svc, err := newRDSQueryClient(cfg)
	if err != nil {
		return nil, region, err
	}
	return &Client{client: svc}, region, nil
}
