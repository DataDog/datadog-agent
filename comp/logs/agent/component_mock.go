// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package agent contains logs agent component.
package agent

import (
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Mock implements mock-specific methods.
type Mock interface {
	Component

	SetSources(sources *sources.LogSources)
}
