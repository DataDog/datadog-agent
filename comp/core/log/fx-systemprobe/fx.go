// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fxlog defines the fx options for this component.
package fxlog

import (
	logimpl "github.com/DataDog/datadog-agent/comp/core/log/impl-systemprobe"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			logimpl.NewComponent,
		),
	)
}
