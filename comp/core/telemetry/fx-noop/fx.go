// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the noop telemetry component (serverless).
package fx

import (
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the noop telemetry component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(noopsimpl.NewComponent),
	)
}
