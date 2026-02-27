// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && !kubelet

package python

/*
#include <datadog_agent_rtloader.h>
#cgo !aix,!windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo aix LDFLAGS: -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static
*/
import "C"

// GetKubeletConnectionInfo writes the kubelet connection info to the payload
//
//export GetKubeletConnectionInfo
func GetKubeletConnectionInfo(payload **C.char) {
	*payload = TrackedCString("{}")
}
