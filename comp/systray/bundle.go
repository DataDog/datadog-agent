// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package systray implements the "systray" bundle for the systray app.
//
// Including `systray.Module` in an App will automatically start the app.
//
// This bundle depends on comp/core.
package systray

import (
	"github.com/DataDog/datadog-agent/comp/systray/systray"
	"github.com/DataDog/datadog-agent/comp/systray/internal"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: agent-windows

// BundleParams defines the parameters for this bundle
type BundleParams = internal.BundleParams

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	systray.Module,

	// require the systray component, causing it to start
	fx.Invoke(func(_ systray.Component) {}),
)


