// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package runtime

import (
	"sync/atomic"
)

var pythonMemoryInuse atomic.Uint64

// SetPythonMemoryInUse sets the current memory in use by Python
func SetPythonMemoryInUse(inuse uint64) {
	pythonMemoryInuse.Store(inuse)
}
