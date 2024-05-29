// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package fx

import (
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metrics-logs

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			replay.NewTrafficCapture,
		),
	)
}
