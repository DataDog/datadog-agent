// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package gpusubscriber subscribes to GPU events
package gpusubscriber

import (
	gsdef "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
)

// Mock implements mock-specific methods.
type Mock interface {
	gsdef.Component
}
