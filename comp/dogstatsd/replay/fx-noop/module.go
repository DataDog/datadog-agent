// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fxnoop provides an fx module for the noop implementation.
package fxnoop

import (
	replaynoop "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/impl-noop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the noop component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			replaynoop.NewNoopTrafficCapture,
		),
	)
}
