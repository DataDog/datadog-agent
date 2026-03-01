// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package statsd

import (
	statsdimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
// Deprecated: Use comp/dogstatsd/statsd/impl.MockModule() instead.
func MockModule() fxutil.Module {
	return statsdimpl.MockModule()
}

// MockClient is an alias for injecting a mock client.
// Deprecated: Use comp/dogstatsd/statsd/impl.MockClient instead.
type MockClient = statsdimpl.MockClient
