// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package rcstatusfx provides the fx module for the rcstatus component.
package rcstatusfx

import (
	rcstatusimpl "github.com/DataDog/datadog-agent/comp/remote-config/rcstatus/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the rcstatus component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(rcstatusimpl.NewStatus),
	)
}
