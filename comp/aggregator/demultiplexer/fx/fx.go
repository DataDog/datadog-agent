// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the demultiplexer component
package fx

import (
	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params demultiplexerimpl.Params) fxutil.Module {
	return demultiplexerimpl.Module(params)
}
