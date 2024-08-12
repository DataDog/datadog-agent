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

// GetECSAgentVersion fetches the ECS Agent Version if running in ECS
func GetECSAgentVersion(_ context.Context) string {
	return ""
}
