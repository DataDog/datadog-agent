// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides the gpu subscriber mock component for the Datadog Agent
package mock

import (
	gsdef "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
)

// Mock implements mock-specific methods.
type Mock interface {
	gsdef.Component
}
