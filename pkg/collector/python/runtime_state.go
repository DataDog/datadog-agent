// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import "sync"

var (
	pythonRuntimeAvailable = true
	pythonRuntimeMu        sync.RWMutex
)

func setPythonRuntimeAvailable(v bool) {
	pythonRuntimeMu.Lock()
	pythonRuntimeAvailable = v
	pythonRuntimeMu.Unlock()
}

func IsPythonRuntimeAvailable() bool {
	pythonRuntimeMu.RLock()
	available := pythonRuntimeAvailable
	pythonRuntimeMu.RUnlock()
	return available
}
