// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"sync"
)

// COREResult enumerates CO-RE success & failure modes
type COREResult int

const (
	successCustomBTF COREResult = iota
	successEmbeddedBTF
	successDefaultBTF
	btfNotFound
	AssetReadError
	VerifierError
	LoaderError
)

// coreTelemetryByAsset is a global object which is responsible for storing CO-RE telemetry for all ebpf assets
var coreTelemetryByAsset = make(map[string]COREResult)
var telemetrymu sync.Mutex

// StoreCORETelemetryForAsset stores CO-RE telemetry for a particular asset.
// If NPM is enabled, all stored telemetry will be sent to the backend as part of the agent payload & emitted internally.
func StoreCORETelemetryForAsset(assetName string, result COREResult) {
	telemetrymu.Lock()
	defer telemetrymu.Unlock()

	coreTelemetryByAsset[assetName] = result
}

// GetCORETelemetryByAsset returns the stored CO-RE telemetry
func GetCORETelemetryByAsset() map[string]int32 {
	telemetrymu.Lock()
	defer telemetrymu.Unlock()

	result := make(map[string]int32)
	for assetName, tm := range coreTelemetryByAsset {
		result[assetName] = int32(tm)
	}
	return result
}
