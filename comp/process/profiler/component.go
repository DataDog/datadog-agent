// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package profiler implements a component to handle starting and stopping the internal profiler.
package profiler

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

type Component interface {
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newProfiler),
)
