// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the collector component.
package mock

import (
	collectorimpl "github.com/DataDog/datadog-agent/comp/collector/collector/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return collectorimpl.MockModule()
}
