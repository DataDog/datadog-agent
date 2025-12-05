// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock for the workloadfilter component
package mock

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// Mock implements mock-specific methods for the workloadfilter component.
type Mock interface {
	workloadfilter.Component
}
