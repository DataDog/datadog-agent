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
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	config.Module,
	coreconfig.Module,
)

// Bundle defines the fx options for this bundle.
var MockBundle = fxutil.Bundle(
	config.MockModule,
	coreconfig.MockModule,
)
