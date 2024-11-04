// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent contains High Availability Agent related code
package haagent

import (
	"go.uber.org/atomic"
)

// TODO: SHOULD BE A COMPONENT WITH STATE

var isPrimaryStore = atomic.NewBool(false)

func ShouldRunHAIntegrationInstance() bool {
	return isPrimaryStore.Load()
}

func SetPrimary(isPrimary bool) {
	isPrimaryStore.Store(isPrimary)
}
