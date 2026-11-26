// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// restoreOverrideFuncsAtCleanup restores, at the end of the test, the override functions registered when it's called
func restoreOverrideFuncsAtCleanup(t testing.TB) {
	model.RestoreOverrideFuncs(t)
}
