// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd //nolint:revive // TODO(AML) Fix revive linter

import (
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metrics-logs

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		serverdebugimpl.Module(),
		replay.Module(),
		pidmapimpl.Module(),
		server.Module())
}

// ClientBundle defines the fx options for this bundle.
var ClientBundle = fxutil.Bundle(
	statsd.Module(),
)
