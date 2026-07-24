// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the experimental Agent lifecycle component.
package fx

import (
	agentlifecycleimpl "github.com/DataDog/datadog-agent/comp/core/agentlifecycle/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the experimental Agent lifecycle component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(agentlifecycleimpl.NewComponent),
	)
}
