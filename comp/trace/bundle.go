// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trace implements the "trace" bundle, providing components for the Trace Agent
//
// The constituent components serve as utilities and are mostly independent of
// one another.  Other components should depend on any components they need.
//
// This bundle does not depend on any other bundles.
package trace

import (
	"github.com/DataDog/datadog-agent/comp/trace/agent"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		config.Module(),
		agent.Module())
}
