// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package expvars initializes the expvar server of the process agent.
package expvars

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

type Component interface {
}

var Module = fxutil.Component(
	fx.Provide(newExpvarServer),
)
