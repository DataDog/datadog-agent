// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the integrations component
package fx

import (
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/impl"
	integrationsmock "github.com/DataDog/datadog-agent/comp/logs/integrations/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			integrations.NewComponent,
		),
	)
}

// MockModule provides the mock integrations component to fx
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			integrationsmock.Mock,
		),
	)
}
