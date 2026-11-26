// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !test

package mock

import "testing"

// restoreOverrideFuncsAtCleanup is a no-op without the test build tag, which model.RestoreOverrideFuncs requires: modules
// importing this package without it, like comp/otelcol/otlp/testutil, keep the override functions across tests.
func restoreOverrideFuncsAtCleanup(testing.TB) {}
