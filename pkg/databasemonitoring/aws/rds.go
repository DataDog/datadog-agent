// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

// Package aws contains database-monitoring specific aurora discovery logic
package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

const (
	postgresqlEngine = "postgresql"
	mysqlEngine      = "mysql"
)

func (c *Client) GetRdsInstancesFromTags(ctx context.Context, tags []string) ([]Instance, error) {
	if len(tags) == 0 {
		return nil, fmt.Errorf("at least one tag filter is required")
	}
	instances := make([]Instance, 0)
	var marker *string
	var err error
	for {
		dbInstances, err := c.client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
			Marker: marker,
			Filters: []types.Filter{
				{
					Name: aws.String("engine"),
					Values: []string{
						mysqlEngine, postgresqlEngine,
					},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("error running GetAuroraClustersFromTags: %v", err)
		}
		for _, db := range dbInstances.DBInstances {
			if containsTags(db.TagList, tags) {
				// Add to list of instances for the cluster
				instance := Instance{
					Endpoint: *db.Endpoint.Address,
				}
				// Set if IAM is configured for the endpoint
				if db.IAMDatabaseAuthenticationEnabled != nil {
					instance.IamEnabled = *db.IAMDatabaseAuthenticationEnabled
				}
				// Set the port, if it is known
				if db.Endpoint.Port != nil {
					instance.Port = *db.Endpoint.Port
				}
				if db.Engine != nil {
					instance.Engine = *db.Engine
				}
				if db.DBName != nil {
					instance.DbName = *db.DBName
				} else {
					if db.Engine != nil {
						defaultDBName, err := dbNameFromEngine(*db.Engine)
						if err != nil {
							return nil, fmt.Errorf("error getting default db name from engine: %v", err)
						}

						instance.DbName = defaultDBName
					} else {
						// This should never happen, as engine is a required field in the API
						// but we should handle it.
						return nil, fmt.Errorf("engine is nil for instance %s", *db.DBInstanceIdentifier)
					}
				}
				instances = append(instances, instance)
			}
		}
		marker = dbInstances.Marker
		if marker == nil {
			break
		}
	}

	return instances, err
}
