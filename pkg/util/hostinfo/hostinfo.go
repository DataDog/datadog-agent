// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
// # Compatibility
//
// This module is exported and can be used outside of the datadog-agent
// repository, but is not designed as a general-purpose logging system.  Its
// API may change incompatibly.

// Package hostinfo helps collect relevant host information
package hostinfo

import (
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

var hostInfoCacheKey = cache.BuildAgentKey("host", "utils", "hostInfo")
