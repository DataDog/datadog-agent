// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

// Package ecs provides information about the ECS Agent Version when running in ECS
package ecs

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
)

var metaV1 v1.Client
var err error

// MetaECS stores ECS metadata to be exported to a json file in the agent flare
type MetaECS struct {
	ECSCluster      string
	ECSAgentVersion string
}

// NewECSMeta returns an ECSConfig object
func NewECSMeta(ctx context.Context) (*MetaECS, error) {
	cluster, version, err := GetECSInstanceMetadata(ctx)
	if err != nil {
		return nil, err
	}

	ecsMeta := MetaECS{
		ECSCluster:      cluster,
		ECSAgentVersion: version,
	}
	return &ecsMeta, nil
}

// GetECSInstanceMetadata fetches the ECS Instance metadata if running in ECS
func GetECSInstanceMetadata(ctx context.Context) (string, string, error) {
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
