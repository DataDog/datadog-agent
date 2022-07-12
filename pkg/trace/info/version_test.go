// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package info

import (
	"testing"
)

func TestPublishVersion(t *testing.T) {
	Version = "v"
	GitCommit = "gc"
	GitBranch = "gb"
	BuildDate = "bd"
	GoVersion = "gv"

	testExpvarPublish(t, publishVersion,
		map[string]interface{}{
			"Version":   "v",
			"GitCommit": "gc",
			"GitBranch": "gb",
			"BuildDate": "bd",
			"GoVersion": "gv",
		})
}
