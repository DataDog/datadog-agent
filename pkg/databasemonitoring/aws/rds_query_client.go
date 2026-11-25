// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"

	awsquery "github.com/DataDog/datadog-agent/pkg/util/aws/query"
)

const (
	rdsServiceName = "rds"
	rdsSDKID       = "RDS"
	rdsAPIVersion  = "2014-10-31"
)

// Tag represents an RDS resource tag.
type Tag struct {
	Key   *string `xml:"Key"`
	Value *string `xml:"Value"`
}

// Endpoint represents a database instance connection endpoint.
type Endpoint struct {
	Address *string `xml:"Address"`
	Port    *int32  `xml:"Port"`
}

// DBCluster represents an Aurora/RDS DB cluster returned by DescribeDBClusters.
type DBCluster struct {
	DBClusterIdentifier *string `xml:"DBClusterIdentifier"`
	TagList             []Tag   `xml:"TagList>Tag"`
}

// DBInstance represents an RDS/Aurora DB instance returned by DescribeDBInstances.
type DBInstance struct {
	Endpoint                         *Endpoint `xml:"Endpoint"`
	DBInstanceIdentifier             *string   `xml:"DBInstanceIdentifier"`
	DBClusterIdentifier              *string   `xml:"DBClusterIdentifier"`
	IAMDatabaseAuthenticationEnabled *bool     `xml:"IAMDatabaseAuthenticationEnabled"`
	AvailabilityZone                 *string   `xml:"AvailabilityZone"`
	DBInstanceStatus                 *string   `xml:"DBInstanceStatus"`
	Engine                           *string   `xml:"Engine"`
	DBName                           *string   `xml:"DBName"`
	TagList                          []Tag     `xml:"TagList>Tag"`
}

// Filter represents a filter passed to RDS DescribeDBInstances or DescribeDBClusters.
type Filter struct {
	Name   *string
	Values []string
}

// DescribeDBInstancesInput holds the parameters for RDS DescribeDBInstances.
type DescribeDBInstancesInput struct {
	Filters []Filter
	Marker  *string
}

// DescribeDBInstancesOutput holds the XML response of RDS DescribeDBInstances.
type DescribeDBInstancesOutput struct {
	DBInstances []DBInstance `xml:"DescribeDBInstancesResult>DBInstances>DBInstance"`
	Marker      *string      `xml:"DescribeDBInstancesResult>Marker"`
}

// DescribeDBClustersInput holds the parameters for RDS DescribeDBClusters.
type DescribeDBClustersInput struct {
	Filters []Filter
	Marker  *string
}

// DescribeDBClustersOutput holds the XML response of RDS DescribeDBClusters.
type DescribeDBClustersOutput struct {
	DBClusters []DBCluster `xml:"DescribeDBClustersResult>DBClusters>DBCluster"`
	Marker     *string     `xml:"DescribeDBClustersResult>Marker"`
}

type rdsQueryClient struct {
	client *awsquery.Client
}

func newRDSQueryClient(cfg aws.Config) (*rdsQueryClient, error) {
	c, err := awsquery.NewClient(cfg, rdsServiceName, rdsSDKID, rdsAPIVersion)
	if err != nil {
		return nil, err
	}
	return &rdsQueryClient{client: c}, nil
}

func (c *rdsQueryClient) DescribeDBInstances(ctx context.Context, params *DescribeDBInstancesInput) (*DescribeDBInstancesOutput, error) {
	form := url.Values{}
	if params != nil {
		if params.Marker != nil && *params.Marker != "" {
			form.Set("Marker", *params.Marker)
		}
		encodeRDSFilters(form, params.Filters)
	}

	var out DescribeDBInstancesOutput
	if err := c.client.Do(ctx, "DescribeDBInstances", form, &out); err != nil {
		return nil, err
	}
	if out.Marker != nil && *out.Marker == "" {
		out.Marker = nil
	}
	return &out, nil
}

func (c *rdsQueryClient) DescribeDBClusters(ctx context.Context, params *DescribeDBClustersInput) (*DescribeDBClustersOutput, error) {
	form := url.Values{}
	if params != nil {
		if params.Marker != nil && *params.Marker != "" {
			form.Set("Marker", *params.Marker)
		}
		encodeRDSFilters(form, params.Filters)
	}

	var out DescribeDBClustersOutput
	if err := c.client.Do(ctx, "DescribeDBClusters", form, &out); err != nil {
		return nil, err
	}
	if out.Marker != nil && *out.Marker == "" {
		out.Marker = nil
	}
	return &out, nil
}

func encodeRDSFilters(form url.Values, filters []Filter) {
	for i, f := range filters {
		prefix := fmt.Sprintf("Filters.Filter.%d", i+1)
		if f.Name != nil {
			form.Set(prefix+".Name", *f.Name)
		}
		for j, v := range f.Values {
			form.Set(fmt.Sprintf("%s.Values.Value.%d", prefix, j+1), v)
		}
	}
}
