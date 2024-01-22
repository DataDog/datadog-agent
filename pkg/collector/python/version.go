// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package python

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

var (
	pythonInfoCacheKey = cache.BuildAgentKey("pythonInfo")
)

// GetPythonInfo returns the info string as provided by the embedded Python interpreter.
//
// Example: '3.10.6 (main, May 29 2023, 11:10:38) [GCC 11.3.0]'
func GetPythonInfo() string {
	// retrieve the Python version from the Agent cache
	if x, found := cache.Cache.Get(pythonInfoCacheKey); found {
		return x.(string)
	}

	return "n/a"
}

// GetPythonVersion returns the embedded python version as provided by the embedded Python interpreter.
//
// Example: '3.10.6'
func GetPythonVersion() string {
	return strings.SplitN(GetPythonInfo(), " ", 2)[0]
}
