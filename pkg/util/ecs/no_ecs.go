// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !docker

// Package ecs provides information about the ECS Agent Version when running in ECS
package ecs

// MetaECS stores ECS cluster metadata
type MetaECS struct {
	AWSAccountID    string
	Region          string
	ECSCluster      string
	ECSClusterID    string
	ECSAgentVersion string
}

// GetClusterMeta returns the cluster meta for ECS.
func GetClusterMeta() (*MetaECS, error) {
	return &MetaECS{}, nil
}
