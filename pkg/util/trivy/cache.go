// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

package trivy

import (
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/aquasecurity/trivy/pkg/utils"
)

type CacheProvider func() (cache.Cache, error)

func NewLocalCache(cacheDir string) (cache.Cache, error) {
	if cacheDir == "" {
		cacheDir = utils.DefaultCacheDir()
	}

	return cache.NewFSCache(cacheDir)
}
