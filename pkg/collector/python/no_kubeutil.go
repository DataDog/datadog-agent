// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python,!kubelet

package python

/*
#include <datadog_agent_six.h>
#cgo !windows LDFLAGS: -ldatadog-agent-six -ldl
#cgo windows LDFLAGS: -ldatadog-agent-six -lstdc++ -static
*/
import "C"

//export GetKubeletConnectionInfo
func GetKubeletConnectionInfo(payload **C.char) {
	*payload = C.CString("{}")
}
