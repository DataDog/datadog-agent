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
	Engine     string
}

const (
	auroraPostgresqlEngine = "aurora-postgresql"
	auroraMysqlEngine      = "aurora-mysql"
)

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
		return nil, fmt.Errorf("error running GetAuroraClusterEndpoints %v", err)
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
			if db.Engine != nil {
				instance.Engine = *db.Engine
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

// GetAuroraClustersFromTags returns a list of Aurora clusters to query from a list of tags
// it is required to query for the cluster ids first because tags are not propagated to instances
// that are brought up during an auto-scaling event. That means the only way to reliably filter for the list
// of database instances is to first query for the cluster ids. This also means the customer
// will only have to set a single tag on their cluster.
func (c *Client) GetAuroraClustersFromTags(ctx context.Context, tags []string) ([]string, error) {
	if len(tags) == 0 {
		return nil, fmt.Errorf("at least one tag filter is required")
	}
	clusterIdentifiers := make([]string, 0)
	clusters, err := c.client.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
		Filters: []types.Filter{
			{
				Name: aws.String("engine"),
				Values: []string{
					auroraMysqlEngine, auroraPostgresqlEngine,
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error running GetAuroraClustersFromTags: %v", err)
	}
	for _, cluster := range clusters.DBClusters {
		if cluster.DBClusterIdentifier != nil && containsTags(cluster.TagList, tags) {
			clusterIdentifiers = append(clusterIdentifiers, *cluster.DBClusterIdentifier)
		}
	}
	return clusterIdentifiers, err
}

func containsTags(clusterTags []types.Tag, providedTags []string) bool {
	pTagMap := make(map[string]struct{}, len(providedTags))
	for _, tag := range providedTags {
		pTagMap[strings.ToLower(tag)] = struct{}{}
	}
	cTagMap := make(map[string]struct{}, len(clusterTags))
	for _, tag := range clusterTags {
		if tag.Key != nil && tag.Value != nil {
			key := strings.ToLower(fmt.Sprintf("%s:%s", *tag.Key, *tag.Value))
			cTagMap[key] = struct{}{}
		}
	}
	// check if all values in pTagMap exist in cTagMap
	for tag := range pTagMap {
		if _, ok := cTagMap[tag]; !ok {
			return false
		}
	}
	return true
}

// Digest returns a hash value representing the data stored in this configuration, minus the network address
func (c *Instance) Digest(checkType, clusterID string) string {
	h := fnv.New64()
	// Hash write never returns an error
	h.Write([]byte(checkType))                       //nolint:errcheck
	h.Write([]byte(clusterID))                       //nolint:errcheck
	h.Write([]byte(c.Endpoint))                      //nolint:errcheck
	h.Write([]byte(fmt.Sprintf("%d", c.Port)))       //nolint:errcheck
	h.Write([]byte(c.Engine))                        //nolint:errcheck
	h.Write([]byte(fmt.Sprintf("%t", c.IamEnabled))) //nolint:errcheck

	return strconv.FormatUint(h.Sum64(), 16)
}
