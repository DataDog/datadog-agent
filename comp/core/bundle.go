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
package core

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/internal"
)

// team: agent-shared-components

const componentName = "comp/core"

// BundleParams defines the parameters for this bundle.
type BundleParams = internal.BundleParams

// Bundle defines the fx options for this bundle.
var Bundle = fx.Module(
	componentName,

	config.Module,
)

// MockBundle defines the mock fx options for this bundle.
var MockBundle = fx.Module(
	componentName,

	config.MockModule,
)
