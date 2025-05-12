// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build rds

package rds

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

// Client is a wrapper around the AWS RDS client.
type Client struct {
	rdsClient *rds.RDS
}

// NewClient creates a new RDS client.
func NewClient(region string) (*Client, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &Client{
		rdsClient: rds.New(sess),
	}, nil
}

// ListInstances fetches a list of RDS instances.
func (c *Client) ListInstances(ctx context.Context) ([]*rds.DBInstance, error) {
	input := &rds.DescribeDBInstancesInput{}
	var instances []*rds.DBInstance

	err := c.rdsClient.DescribeDBInstancesPagesWithContext(ctx, input, func(page *rds.DescribeDBInstancesOutput, lastPage bool) bool {
		instances = append(instances, page.DBInstances...)
		return !lastPage
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe RDS instances: %w", err)
	}

	return instances, nil
}
