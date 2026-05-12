// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the health platform check runner component.
package fx

import (
	checkrunnerimpl "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the check runner component.
// The reporter is wired separately via SetReporter() in the lifecycle start hook
// to avoid a circular dependency with the core health platform component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(checkrunnerimpl.New),
	)
}
