// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kernel

import (
	"runtime"
	"sync"
)

// Release is an empty string on Windows
var Release = sync.OnceValues(func() (string, error) {
	// this is matching the current behavior of hostinfo.GetInformation().KernelVersion
	return "", nil
})

// Machine is equivalent to runtime.GOARCH on Windows
var Machine = sync.OnceValues(func() (string, error) {
	return runtime.GOARCH, nil
})
