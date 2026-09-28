// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder && python

// Package fx provides the recorder module for the tagged Agent build.
package fx

import (
	recorderimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the Fx options for the Parquet recorder.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(recorderimpl.NewComponent),
	)
}
