// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the hostname component.
package fx

import (
	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	hostnameimpl "github.com/DataDog/datadog-agent/comp/core/hostname/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the hostname component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			hostnameimpl.NewComponent,
		),
		fxutil.ProvideOptional[hostnamedef.Component](),
	)
}
