// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx creates the modules for fx
package fx

import (
	statusimpl "github.com/DataDog/datadog-agent/comp/procmgr/status/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module adds the Process Manager section to agent status.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			statusimpl.NewComponent,
		),
	)
}
