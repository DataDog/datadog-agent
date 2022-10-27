// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"sync"
)

// CoReResult enumerates CO-RE success & failure modes
type CoReResult int

const (
	successCustomBTF CoReResult = iota
	successEmbeddedBTF
	successDefaultBTF
	btfNotFound
	AssetReadError
	VerifierError
)

// coReTelemetryByAsset is a global object which is responsible for storing CO-RE telemetry for all ebpf assets
var coReTelemetryByAsset = make(map[string]CoReResult)
var telemetrymu sync.Mutex

// StoreCoReTelemetryForAsset stores CO-RE telemetry for a particular asset.
// If NPM is enabled, all stored telemetry will be sent to the backend as part of the agent payload & emitted internally.
func StoreCoReTelemetryForAsset(assetName string, result CoReResult) {
	telemetrymu.Lock()
	defer telemetrymu.Unlock()

	coReTelemetryByAsset[assetName] = result
}

// GetCoReTelemetryByAsset returns the stored CO-RE telemetry
func GetCoReTelemetryByAsset() map[string]int32 {
	telemetrymu.Lock()
	defer telemetrymu.Unlock()

	result := make(map[string]int32)
	for assetName, tm := range coReTelemetryByAsset {
		result[assetName] = int32(tm)
	}
	return result
}
