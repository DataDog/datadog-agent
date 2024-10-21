// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resolvers holds resolvers related files
package resolvers

import "github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"

// Opts defines common options
type Opts struct {
	PathResolutionEnabled    bool
	EnvVarsResolutionEnabled bool
	TagsResolver             tags.Resolver
	UseRingBuffer            bool
	TTYFallbackEnabled       bool
}
