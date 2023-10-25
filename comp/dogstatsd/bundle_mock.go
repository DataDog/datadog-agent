// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockBundle defines the mock fx options for this bundle.
var MockBundle = fxutil.Bundle(
	serverdebugimpl.MockModule,
	server.MockModule,
	replay.Module,
)
