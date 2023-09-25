// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

// Package process holds process related files
package process

import "go.uber.org/atomic"

// Resolver defines a resolver
type Resolver struct {
	// stats
	cacheSize *atomic.Int64
}
