// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !python

// Package fx provides the fx module for the Data Observability query actions component
package fx

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module is a no-op for builds without Python support (e.g. IoT agent), which do not support Data Observability query actions.
func Module() fxutil.Module {
	return fxutil.Module{Option: fx.Options()}
}
