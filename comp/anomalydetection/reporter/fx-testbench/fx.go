// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the testbench fx module for the reporter component.
// Wire this in the testbench binary: it provides the SSE-pushing reporter.Reporter
// and exposes SSEAccess for the HTTP API to register browser clients.
package fx

import (
	testbenchimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl-testbench"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the testbench reporter component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(testbenchimpl.NewComponent),
	)
}
