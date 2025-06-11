// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

const (
	postgresEngine = "postgres"
	mysqlEngine    = "mysql"
)

// GetRdsInstancesFromTags queries an AWS account for RDS instances with the specified tags
func (c *Client) GetRdsInstancesFromTags(ctx context.Context, tags []string, dbmTag string) ([]Instance, error) {
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
						mysqlEngine, postgresEngine,
						// Allow RDS to return Aurora instances as well
						auroraMysqlEngine, auroraPostgresqlEngine,
					},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("error running GetRdsInstancesFromTags: %v", err)
		}
		for _, db := range dbInstances.DBInstances {
			if containsTags(db.TagList, tags) {
				instance, err := makeInstance(db, dbmTag)
				if err != nil {
					log.Errorf("error creating instance from DBInstance: %v", err)
					continue
				}
				if instance != nil {
					instances = append(instances, *instance)
				}
			}
		}
		marker = dbInstances.Marker
		if marker == nil {
			break
		}
	}

	return instances, err
}
