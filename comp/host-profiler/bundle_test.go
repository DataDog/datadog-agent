// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package hostprofiler

import (
	"testing"

	collectorimpl "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// TestBundleDependencies tests that the bundle can be created without errors.
//
// This test ensures that all dependencies in the host profiler bundle are
// properly configured and can be resolved by the dependency injection container.
func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t,
		Bundle(collectorimpl.NewParams("", false)),
		fx.Provide(collectorimpl.NewExtraFactoriesWithoutAgentCore),
	)
}
