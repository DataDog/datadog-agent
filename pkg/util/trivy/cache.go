// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

package trivy

import (
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/aquasecurity/trivy/pkg/utils"
)

// telemetryTick is the frequency at which the cache usage metrics are collected.
var telemetryTick = 1 * time.Minute

// CacheProvider describe a function that provides a type implementing the trivy cache interface
// and a cache cleaner
type CacheProvider func() (cache.Cache, CacheCleaner, error)

// NewBoltCache is a CacheProvider. It returns a BoltDB cache provided by Trivy and an empty cleaner.
func NewBoltCache(cacheDir string) (cache.Cache, CacheCleaner, error) {
	if cacheDir == "" {
		cacheDir = utils.DefaultCacheDir()
	}
	cache, err := cache.NewFSCache(cacheDir)
	return cache, &StubCacheCleaner{}, err
}

// StubCacheCleaner is a stub
type StubCacheCleaner struct{}

// Clean does nothing
func (c *StubCacheCleaner) Clean() error { return nil }
