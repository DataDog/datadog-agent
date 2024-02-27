// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"time"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=rdsclient_mockgen.go

// RDSClient is the interface for describing aurora cluster endpoints
type RDSClient interface {
	GetAuroraClusterEndpoints(ctx context.Context, dbClusterIdentifiers []string) (map[string]*AuroraCluster, error)
	GetAuroraClustersFromTags(ctx context.Context, tags []string) ([]string, error)
}

// rdsService defines the interface for describing cluster instances. It exists here to facilitate testing
// but the *rds.Client will be the implementation for production code.
type rdsService interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	DescribeDBClusters(ctx context.Context, params *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
}

// Client is a wrapper around the AWS RDS client
type Client struct {
	client rdsService
}

// NewRDSClient creates a new AWS client for querying RDS
func NewRDSClient() (*Client, string, error) {
	// get region from instance metadata
	identity, err := ec2.GetInstanceIdentity(context.Background())
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Try to load shared AWS configuration.
	// The default configuration sources are:
	// * Environment Variables
	// * Shared Configuration and Shared Credentials files.
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(identity.Region),
	)
	if err != nil {
		return nil, identity.Region, err
	}

	svc := rds.NewFromConfig(cfg)
	return &Client{client: svc}, identity.Region, nil
}
