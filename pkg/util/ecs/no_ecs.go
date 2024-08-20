// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !docker

// Package ecs provides information about the ECS Agent Version when running in ECS
package ecs

import (
	"context"
)

// MetaECS stores ECS metadata to be exported to a json file in the agent flare
type MetaECS struct{}

// NewECSMeta returns an ECSConfig object
func NewECSMeta(ctx context.Context) (*MetaECS, error) {
	return nil, nil
}

// GetECSInstanceMetadata fetches the ECS Instance metadata if running in ECS
func GetECSInstanceMetadata(ctx context.Context) (string, string, error) {
	return "", "", nil
}
