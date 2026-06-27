// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package utils

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// pythonInfoCacheKey matches the key used by pkg/collector/python to store the
// Python interpreter info string in the shared agent cache. We read from the cache
// directly to avoid importing the CGo-heavy pkg/collector/python package here,
// which would pull in rtloader build constraints into the host metadata component.
var pythonInfoCacheKey = cache.BuildAgentKey("pythonInfo")

func getPythonInfo() string {
	if x, found := cache.Cache.Get(pythonInfoCacheKey); found {
		return x.(string)
	}
	return "n/a"
}

func getPythonVersion() string {
	return strings.SplitN(getPythonInfo(), " ", 2)[0]
}
