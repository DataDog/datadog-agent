// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build zlib

package status

import (
	"fmt"
)

func getSystemProbeStats() map[string]interface{} {
	return map[string]interface{}{
		"Errors": fmt.Sprintf("System Probe is not supported when built without zstd support"),
	}
}
