// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !docker

package ecs

// IsInstance returns whether this host is part of an ECS cluster
func IsInstance() bool {
	return false
}
