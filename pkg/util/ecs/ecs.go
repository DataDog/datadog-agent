// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

// Package ecs provides information about the ECS Agent Version when running in ECS
package ecs

import (
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

var metaV1 v1.Client
var metaV3or4 v3or4.Client
var err error

// MetaECS stores ECS metadata to be exported to a json file in the agent flare
type MetaECS struct {
	ECSCluster      string
	ECSAgentVersion string
}

// NewECSMeta returns a MetaECS object
func NewECSMeta(ctx context.Context) (*MetaECS, error) {
	var cluster, version string

	if env.IsFeaturePresent(env.ECSFargate) {
		// There is no instance metadata endpoint on ECS Fargate
		cluster, version, err = getECSTaskMetadata(ctx)
	} else {
		cluster, version, err = getECSInstanceMetadata(ctx)
	}

	if err != nil {
		return nil, err
	}

	ecsMeta := MetaECS{
		ECSCluster:      cluster,
		ECSAgentVersion: version,
	}
	return &ecsMeta, nil
}

func getECSInstanceMetadata(ctx context.Context) (string, string, error) {
	metaV1, err = metadata.V1()
	if err != nil {
		return "", "", err
	}

	ecsInstance, err := metaV1.GetInstance(ctx)
	if err != nil {
		return "", "", err
	}

	return ecsInstance.Cluster, ecsInstance.Version, err
}

func getECSTaskMetadata(ctx context.Context) (string, string, error) {
	metaV3or4, err = metadata.V3orV4FromCurrentTask()
	if err != nil {
		return "", "", err
	}

	ecsTask, err := metaV3or4.GetTask(ctx)
	if err != nil {
		return "", "", err
	}

	return ParseClusterName(ecsTask.ClusterName), ecsTask.Version, err
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
