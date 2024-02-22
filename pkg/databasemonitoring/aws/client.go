// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package aws

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"time"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=rdsclient_mockgen.go

// RDSClient is the interface for describing aurora cluster endpoints
type RDSClient interface {
	GetAuroraClusterEndpoints(ctx context.Context, dbClusterIdentifiers []string) (map[string]*AuroraCluster, error)
}

// rdsService defines the interface for describing cluster instances. It exists here to facilitate testing
// but the *rds.Client will be the implementation for production code.
type rdsService interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

// Client is a wrapper around the AWS RDS client
type Client struct {
	client rdsService
}

// NewRDSClient creates a new AWS client for querying RDS
func NewRDSClient(region, roleArn string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return nil, err
	}

	stsClient := sts.NewFromConfig(cfg)
	provider := stscreds.NewAssumeRoleProvider(stsClient, roleArn)
	cfg.Credentials = aws.NewCredentialsCache(provider)

	rdsSvc := rds.NewFromConfig(cfg)
	return &Client{client: rdsSvc}, nil
}
