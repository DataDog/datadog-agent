// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"time"

	"github.com/patrickmn/go-cache"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

const (
	defaultCacheExpire = 30 * time.Second
	defaultCachePurge  = 2 * time.Minute
)

// NewRemoteProgram creates a new remote program.
func NewRemoteProgram(name string, objectType workloadfilter.ResourceType, builder *ProgramBuilder, provider program.ClientProvider) program.FilterProgram {
	return &program.RemoteProgram{
		Name:           name,
		ObjectType:     string(objectType),
		Logger:         builder.logger,
		TelemetryStore: builder.telemetryStore,
		Cache:          cache.New(defaultCacheExpire, defaultCachePurge),
		Provider:       provider,
	}
}
