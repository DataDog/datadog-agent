// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package statsd

import (
	statsdimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/impl"
)

// Mock implements mock-specific methods.
// Deprecated: Use comp/dogstatsd/statsd/impl.Mock directly.
type Mock = statsdimpl.Mock
