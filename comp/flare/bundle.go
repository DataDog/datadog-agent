// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements the "flare" bundle, providing services for flares to all
// agent flavors and binaries.
//
// The constituent components serve as utilities and are mostly independent of
// one another. Other components should depend on any components they need.
//
// This Bundle should be part of the "core" bundle. But linking again the "pkg/flare" package adds 30MB to the binaries.
// For this reason we moved this to it's own bundle for now. Once "pkg/flare" has fully been migrated we will merge this
// bundle with the "core" bundle.
//
// This bundle depends on the "core" bundle.
package flare

import (
	"github.com/DataDog/datadog-agent/comp/flare/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	flare.Module,
)

// MockBundle defines the mock fx options for this bundle.
var MockBundle = fxutil.Bundle(
	flare.Module,
)
