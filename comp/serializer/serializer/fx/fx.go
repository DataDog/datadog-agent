// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the serializer component.
package fx

import (
	serializerimpl "github.com/DataDog/datadog-agent/comp/serializer/serializer/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the serializer component and all of its
// functional group components.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(serializerimpl.NewComponent),
	)
}
