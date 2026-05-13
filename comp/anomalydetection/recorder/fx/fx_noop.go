// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

// Package fx provides the fx module for the recorder component.
package fx

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module is a no-op for builds without Python support (e.g. IoT agent).
// Returning empty fx options avoids linking apache/arrow-go into binaries
// that do not need parquet recording. Consumers of recorder.Component are
// expected to gate on the same `python` tag — see observer/fx for the
// matching shape that ensures observer's optional Recorder dependency is
// only declared when the real recorder Module is wired.
func Module() fxutil.Module {
	return fxutil.Module{Option: fx.Options()}
}
