// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import "runtime/debug"

// releaseMemory releases memory to the OS
func releaseMemory() {
	// Release the memory garbage collected by the Go runtime to OS
	debug.FreeOSMemory()
}
