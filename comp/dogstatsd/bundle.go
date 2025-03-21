// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd //nolint:revive // TODO(AML) Fix revive linter

import (
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	replayfx "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metric-pipelines

// Bundle defines the fx options for this bundle.
func Bundle(params server.Params) fxutil.BundleOptions {
	return fxutil.Bundle(
		serverdebugimpl.Module(),
		replayfx.Module(),
		pidmapimpl.Module(),
		server.Module(params))
}
