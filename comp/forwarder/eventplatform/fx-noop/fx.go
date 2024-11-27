// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fxnoop module for the rdnsquerier component
package fx

import (
	eventplatformnoop "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl-noop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			eventplatformnoop.NewComponent,
		),
	)
}
