// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !python

// Package anomalysampler is a no-op on builds without Python support (e.g. IoT agent).
package anomalysampler

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module is a no-op for builds without Python support.
func Module() fxutil.Module {
	return fxutil.Module{Option: fx.Options()}
}
