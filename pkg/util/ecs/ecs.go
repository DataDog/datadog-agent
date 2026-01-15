// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

// Package ecs provides information about the ECS Agent Version when running in ECS
package ecs

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
)

// MetaECS stores ECS cluster metadata
type MetaECS struct {
	AWSAccountID    string
	Region          string
	ECSCluster      string
	ECSClusterID    string
	ECSAgentVersion string
}

func (m *MetaECS) toCacheValue() string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", m.AWSAccountID, m.Region, m.ECSCluster, m.ECSClusterID, m.ECSAgentVersion)
}

func (m *MetaECS) fromCacheValue(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 5 {
		return fmt.Errorf("invalid cache value: %s", value)
	}
	m.AWSAccountID = parts[0]
	m.Region = parts[1]
	m.ECSCluster = parts[2]
	m.ECSClusterID = parts[3]
	m.ECSAgentVersion = parts[4]
	return nil
}

// GetClusterMeta returns the cluster meta for ECS.
func GetClusterMeta() (*MetaECS, error) {
	// Try to get the cluster meta from cache
	meta := &MetaECS{}
	cacheClusterMetaKey := cache.BuildAgentKey(constants.ECSClusterMetaCacheKey)
	if cachedClusterID, found := cache.Cache.Get(cacheClusterMetaKey); found {
		err := meta.fromCacheValue(cachedClusterID.(string))
		if err != nil {
			return nil, err
		}
		return meta, nil
	}

	// If not in cache, get the cluster meta from ECS
	meta, err := newECSMeta(context.Background())
	if err != nil || meta == nil {
		return nil, err
	}

	// Set the cluster meta in cache
	cache.Cache.Set(cacheClusterMetaKey, meta.toCacheValue(), cache.NoExpiration)
	return meta, nil
}

// newECSMeta returns a MetaECS object
func newECSMeta(ctx context.Context) (*MetaECS, error) {
	var awsAccountID, region, cluster, clusterID, version string
	var err error

	if env.IsFeaturePresent(env.ECSFargate) {
		// There is no instance metadata endpoint on ECS Fargate
		awsAccountID, region, cluster, version, err = getECSTaskMetadata(ctx)
	} else {
		awsAccountID, region, cluster, version, err = getECSInstanceMetadata(ctx)
	}

	if err != nil {
		return nil, err
	}

	clusterID, err = initClusterID(awsAccountID, region, cluster)
	if err != nil {
		return nil, err
	}

	ecsMeta := MetaECS{
		AWSAccountID:    awsAccountID,
		Region:          region,
		ECSCluster:      cluster,
		ECSClusterID:    clusterID,
		ECSAgentVersion: version,
	}
	return &ecsMeta, nil
}

func getECSInstanceMetadata(ctx context.Context) (string, string, string, string, error) {
	metaV1, err := metadata.V1()
	if err != nil {
		return "", "", "", "", err
	}

	ecsInstance, err := metaV1.GetInstance(ctx)
	if err != nil {
		return "", "", "", "", err
	}

	region, awsAccountID := ParseRegionAndAWSAccountID(ecsInstance.ContainerInstanceARN)

	return awsAccountID, region, ParseClusterName(ecsInstance.Cluster), ecsInstance.Version, err
}

func getECSTaskMetadata(ctx context.Context) (string, string, string, string, error) {
	metaV3or4, err := metadata.V3orV4FromCurrentTask()
	if err != nil {
		return "", "", "", "", err
	}

	ecsTask, err := metaV3or4.GetTask(ctx)
	if err != nil {
		return "", "", "", "", err
	}

	region, awsAccountID := ParseRegionAndAWSAccountID(ecsTask.TaskARN)

	return awsAccountID, region, ParseClusterName(ecsTask.ClusterName), ecsTask.Version, err
}

// ParseClusterName returns the short name of an ECS cluster. It detects if the name
// is an ARN and converts it if that's the case.
func ParseClusterName(value string) string {
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		return parts[len(parts)-1]
	}

	return value
}

// ParseRegionAndAWSAccountID parses the region and AWS account ID from a ARN.
func ParseRegionAndAWSAccountID(arn string) (string, string) {
	arnParts := strings.Split(arn, ":")
	if len(arnParts) < 5 {
		return "", ""
	}
	// Accept all valid AWS partitions: aws (commercial), aws-us-gov (GovCloud), aws-cn (China)
	partition := arnParts[1]
	if arnParts[0] != "arn" || (partition != "aws" && partition != "aws-us-gov" && partition != "aws-cn") {
		return "", ""
	}
	region := arnParts[3]
	if strings.Count(region, "-") < 2 {
		region = ""
	}

	id := arnParts[4]
	// aws account id is 12 digits
	// https://docs.aws.amazon.com/accounts/latest/reference/manage-acct-identifiers.html
	if len(id) != 12 {
		return region, ""
	}

	return region, id
}

func initClusterID(awsAccountID string, region, clusterName string) (string, error) {
	cluster := fmt.Sprintf("%s/%s/%s", awsAccountID, region, clusterName)

	hash := md5.New()
	hash.Write([]byte(cluster))
	hashString := hex.EncodeToString(hash.Sum(nil))
	id, err := uuid.FromBytes([]byte(hashString[0:16]))
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
