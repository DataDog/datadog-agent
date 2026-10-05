// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pythonruntime defines the collector's boundary for initializing the
// embedded Python runtime.
package pythonruntime

import healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"

// Runtime manages the embedded Python runtime used by Python checks.
type Runtime interface {
	SetHealthPlatform(healthplatform.Component)
	InitPython(paths ...string)
	TerminateRunningProcesses()
}
