// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package aws contains database-monitoring specific aurora discovery logic
package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"hash/fnv"
	"strconv"

	"strings"
)

// AuroraCluster represents an Aurora cluster
type AuroraCluster struct {
	Instances []*Instance
}

// Instance represents an Aurora instance
type Instance struct {
	Endpoint   string
	Port       int32
	IamEnabled bool
}

// GetAuroraClusterEndpoints queries an AWS account for the endpoints of an Aurora cluster
// requires the dbClusterIdentifier for the cluster
func (c *Client) GetAuroraClusterEndpoints(ctx context.Context, dbClusterIdentifiers []string) (map[string]*AuroraCluster, error) {
	if len(dbClusterIdentifiers) == 0 {
		return nil, fmt.Errorf("at least one database cluster identifier is required")
	}
	clusterInstances, err := c.client.DescribeDBInstances(ctx,
		&rds.DescribeDBInstancesInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("db-cluster-id"),
					Values: dbClusterIdentifiers,
				},
			},
		})
	if err != nil {
		return nil, fmt.Errorf("error describing aurora DB clusters: %v", err)
	}
	clusters := make(map[string]*AuroraCluster, 0)
	for _, db := range clusterInstances.DBInstances {
		if db.Endpoint != nil && db.DBClusterIdentifier != nil {
			if db.Endpoint.Address == nil || db.DBInstanceStatus == nil || strings.ToLower(*db.DBInstanceStatus) != "available" {
				continue
			}
			// Add to list of instances for the cluster
			instance := &Instance{
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
			if _, ok := clusters[*db.DBClusterIdentifier]; !ok {
				clusters[*db.DBClusterIdentifier] = &AuroraCluster{
					Instances: make([]*Instance, 0),
				}
			}
			clusters[*db.DBClusterIdentifier].Instances = append(clusters[*db.DBClusterIdentifier].Instances, instance)
		}
	}
	if len(clusters) == 0 {
		return nil, fmt.Errorf("no endpoints found for aurora clusters with id(s): %s", strings.Join(dbClusterIdentifiers, ", "))
	}
	return clusters, nil
}

// Digest returns a hash value representing the data stored in this configuration, minus the network address
func (c *Instance) Digest(checkType, region, clusterID string) string {
	h := fnv.New64()
	// Hash write never returns an error
	h.Write([]byte(checkType))                       //nolint:errcheck
	h.Write([]byte(clusterID))                       //nolint:errcheck
	h.Write([]byte(c.Endpoint))                      //nolint:errcheck
	h.Write([]byte(fmt.Sprintf("%d", c.Port)))       //nolint:errcheck
	h.Write([]byte(region))                          //nolint:errcheck
	h.Write([]byte(fmt.Sprintf("%t", c.IamEnabled))) //nolint:errcheck

	return strconv.FormatUint(h.Sum64(), 16)
}
