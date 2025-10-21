// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package hpflareextensionfx provides fx access for the provider component
package hpflareextensionfx

import (
	extension "github.com/DataDog/datadog-agent/comp/host-profiler/hpflareextension/def"
	extensionimpl "github.com/DataDog/datadog-agent/comp/host-profiler/hpflareextension/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			extensionimpl.NewExtension,
		),
		fxutil.ProvideOptional[extension.Component](),
	)
}
