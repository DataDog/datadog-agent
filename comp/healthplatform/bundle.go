// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatform implements the "healthplatform" bundle, providing the
// health platform component for detecting and reporting agent health issues.
//
// The health platform collects health signals from various agent components,
// persists detected issues, and forwards reports to the Datadog intake.
//
// This bundle does not depend on any other bundles.
package healthplatform

import (
	forwarderfx "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/fx"
	checkrunnerfx "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/fx"
	corefx "github.com/DataDog/datadog-agent/comp/healthplatform/store/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	// Import issue modules to trigger their init() registration.
	// The bundle is the correct place for side-effect imports; impl packages
	// must not import other impl packages.
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/admisconfig"
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/checkfailure"
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/dockerpermissions"
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/rofspermissions"
)

// team: agent-health

// Bundle defines the fx options for the health platform bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		checkrunnerfx.Module(),
		forwarderfx.Module(),
		corefx.Module(),
	)
}
