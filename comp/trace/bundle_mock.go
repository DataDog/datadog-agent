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

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthnoop "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl"
	traceagentfx "github.com/DataDog/datadog-agent/comp/trace/agent/fx-mock"
	traceconfigmock "github.com/DataDog/datadog-agent/comp/trace/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

// MockBundle defines the fx options for this bundle.
func MockBundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		traceconfigmock.MockModule(),
		// The trace agent resolves delegated-auth credential providers, so the mock graph has to
		// supply the component too. The noop never has a credential, which is what a test wants.
		fx.Provide(func() delegatedauth.Component { return delegatedauthnoop.NewComponent().Comp }),
		traceagentfx.MockModule())
}
