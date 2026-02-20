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
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=rdsclient_mockgen.go

// RdsClient is the interface for describing cluster and instance endpoints
type RdsClient interface {
	GetAuroraClusterEndpoints(ctx context.Context, dbClusterIdentifiers []string, config Config) (map[string]*AuroraCluster, error)
	GetAuroraClustersFromTags(ctx context.Context, tags []string) ([]string, error)
	GetRdsInstancesFromTags(ctx context.Context, config Config) ([]Instance, error)
}

// rdsService defines the interface for describing cluster instances. It exists here to facilitate testing
// but the *rds.Client will be the implementation for production code.
type rdsService interface {
	DescribeDBInstances(ctx context.Context, params *awsrds.DescribeDBInstancesInput, optFns ...func(*awsrds.Options)) (*awsrds.DescribeDBInstancesOutput, error)
	DescribeDBClusters(ctx context.Context, params *awsrds.DescribeDBClustersInput, optFns ...func(*awsrds.Options)) (*awsrds.DescribeDBClustersOutput, error)
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

	svc := awsrds.NewFromConfig(cfg)
	return &Client{client: svc}, region, nil
}
