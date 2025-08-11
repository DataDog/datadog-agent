// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package core implements the "core" bundle, providing services common to all
// agent flavors and binaries.
//
// The constituent components serve as utilities and are mostly independent of
// one another.  Other components should depend on any components they need.
//
// This bundle does not depend on any other bundles.

//go:build test

package trace

import (
	"go.uber.org/fx"

	traceagentfx "github.com/DataDog/datadog-agent/comp/trace/agent/fx-mock"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

// MockBundle defines the fx options for this bundle.
func MockBundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		config.MockModule(),
		// Provide nil TracerPayloadModifier by default for tests - can be overridden
		fx.Provide(func() pkgagent.TracerPayloadModifier { return nil }),
		traceagentfx.MockModule())
}
