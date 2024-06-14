// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the logs_from_integrations component
package fx

import (
	logs_from_integrationsimpl "github.com/DataDog/datadog-agent/comp/logs/logs_from_integrations/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			logs_from_integrationsimpl.NewComponent,
		),
	)
}
