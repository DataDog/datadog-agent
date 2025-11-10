// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ssi adds methods to check the status of the APM Single Step Instrumentation
package ssi

// APMInstrumentationStatus contains the instrumentation status of APM Single Step Instrumentation.
type APMInstrumentationStatus struct {
	HostInstrumented   bool `json:"host_instrumented"`
	DockerInstalled    bool `json:"docker_installed"`
	DockerInstrumented bool `json:"docker_instrumented"`
}
