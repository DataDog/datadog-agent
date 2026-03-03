// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

import (
	traceconfigmock "github.com/DataDog/datadog-agent/comp/trace/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock implements mock-specific methods.
// Deprecated: Use comp/trace/config/mock directly.
type Mock interface {
	Component
}

// MockModule defines the fx options for the mock component.
// Deprecated: Use comp/trace/config/mock.MockModule() instead.
func MockModule() fxutil.Module {
	return traceconfigmock.MockModule()
}
