// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the kubehealth component
package fx

import (
	registrarimpl "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module returns the fx module for the kubehealth component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			registrarimpl.NewProvides,
		),
	)
}
