// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"go.uber.org/fx"

	traceagentimpl "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

// Module defines the fx options for the agent component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(traceagentimpl.NewAgent))
}
