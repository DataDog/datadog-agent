// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !docker

package ecs

import "context"

// GetTags returns host tags or static tags that are automatically added to
// ECS metrics
func GetTags(_ context.Context) ([]string, error) { return nil, nil }
