// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx defines the fx options for this component.
package fx

import (
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	impltrace "github.com/DataDog/datadog-agent/comp/core/log/impl-trace"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			impltrace.NewComponent,
		),
		logging.NewFxEventLoggerOption[logdef.Component](),
	)
}
