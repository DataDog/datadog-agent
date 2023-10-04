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
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	// As `config.Module` expects `config.Params` as a parameter, it is require to define how to get `config.Params` from `BundleParams`.
	fx.Provide(func(params BundleParams) config.Params { return params.ConfigParams }),
	config.Module,
	fx.Provide(func(params BundleParams) log.Params { return params.LogParams }),
	log.Module,
	fx.Provide(func(params BundleParams) sysprobeconfig.Params { return params.SysprobeConfigParams }),
	sysprobeconfig.Module,
	telemetry.Module,
	hostname.Module,
)
