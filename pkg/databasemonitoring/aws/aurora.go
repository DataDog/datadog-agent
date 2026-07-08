// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	"strings"
)

const (
	auroraPostgresqlEngine = "aurora-postgresql"
	auroraMysqlEngine      = "aurora-mysql"
)

// GetAuroraClusterEndpoints queries an AWS account for the endpoints of an Aurora cluster
// requires the dbClusterIdentifier for the cluster
func (c *Client) GetAuroraClusterEndpoints(ctx context.Context, clusters []types.DBCluster, config Config) ([]Instance, error) {
	log.Debugf("aurora: getting endpoints for %d clusters", len(clusters))
	if len(clusters) == 0 {
		return nil, nil
	}
	instances := make([]Instance, 0, len(clusters))
	for _, cluster := range clusters {
		// TODO: Seth Samuel: This method is not paginated, so if there are more than 100 instances in a cluster, we will only get the first 100
		// We should add pagination support to this method at some point
		clusterInstances, err := c.client.DescribeDBInstances(ctx,
			&rds.DescribeDBInstancesInput{
				Filters: []types.Filter{
					{
						Name:   aws.String("db-cluster-id"),
						Values: []string{*cluster.DBClusterIdentifier},
					},
				},
			})
		if err != nil {
			return nil, fmt.Errorf("aurora: error running GetAuroraClusterEndpoints %v", err)
		}
		log.Debugf("aurora: found %d instances in cluster %s", len(clusterInstances.DBInstances), *cluster.DBClusterIdentifier)
		for _, db := range clusterInstances.DBInstances {
			if db.Endpoint != nil && db.DBClusterIdentifier != nil {
				if db.Endpoint.Address == nil || db.DBInstanceStatus == nil || strings.ToLower(*db.DBInstanceStatus) != "available" {
					log.Debugf("aurora: skipping instance %v in cluster %s", db, *cluster.DBClusterIdentifier)
					continue
				}
				instance, err := makeInstance(db, &cluster, config)
				log.Debugf("aurora: created instance %v", instance)
				if err != nil || instance == nil {
					log.Errorf("aurora:error creating instance from DBInstance: %v", err)
					continue
				}
				instances = append(instances, *instance)
			}
		}
	}
	return instances, nil
}

// GetAuroraClustersFromTags returns a list of Aurora clusters to query from a list of tags
// it is required to query for the cluster ids first because tags are not propagated to instances
// that are brought up during an auto-scaling event. That means the only way to reliably filter for the list
// of database instances is to first query for the cluster ids. This also means the customer
// will only have to set a single tag on their cluster.
func (c *Client) GetAuroraClustersFromTags(ctx context.Context, tags []string) ([]types.DBCluster, error) {
	clusters := make([]types.DBCluster, 0)
	var marker *string
	var err error
	for {
		clusterDescriptions, err := c.client.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
			Marker: marker,
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
			return nil, fmt.Errorf("aurora: error running GetAuroraClustersFromTags: %v", err)
		}
		log.Debugf("aurora: found %d clusters", len(clusterDescriptions.DBClusters))
		for _, cluster := range clusterDescriptions.DBClusters {
			if cluster.DBClusterIdentifier != nil && containsTags(cluster.TagList, tags) {
				log.Debugf("aurora: found cluster %s", *cluster.DBClusterIdentifier)
				clusters = append(clusters, cluster)
			} else {
				log.Debugf("aurora: skipping cluster %s", *cluster.DBClusterIdentifier)
			}
		}
		marker = clusterDescriptions.Marker
		if marker == nil {
			break
		}
	}

	return clusters, err
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
	h.Write([]byte(checkType))                        //nolint:errcheck
	h.Write([]byte(clusterID))                        //nolint:errcheck
	h.Write([]byte(c.Endpoint))                       //nolint:errcheck
	h.Write([]byte(strconv.Itoa(int(c.Port))))        //nolint:errcheck
	h.Write([]byte(c.Engine))                         //nolint:errcheck
	h.Write([]byte(strconv.FormatBool(c.IamEnabled))) //nolint:errcheck

	return strconv.FormatUint(h.Sum64(), 16)
}
