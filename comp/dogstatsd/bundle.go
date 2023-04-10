// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metrics-logs

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	serverDebug.Module,
	replay.Module,
	server.Module,
)

// MockBundle defines the mock fx options for this bundle.
var MockBundle = fxutil.Bundle(
	serverDebug.MockModule,
	server.MockModule,
	replay.Module,
)
