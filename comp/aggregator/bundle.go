// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package aggregator implements the "aggregator" bundle,
package aggregator

import (
	demultiplexerdef "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	demultiplexerfx "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metric-pipelines

// Bundle defines the fx options for this bundle.
func Bundle(params demultiplexerdef.Params) fxutil.BundleOptions {
	return fxutil.Bundle(
		demultiplexerfx.Module(params))
}
