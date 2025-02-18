// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package gpusubscriberimpl implements a component to handle GPU detection in the Core Agent.
package gpusubscriberimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/process/gpusubscriber"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(fx.Provide(newGpuSubscriber))
}

func newGpuSubscriber() gpusubscriber.Component {
	return NoopSubscriber{}
}
