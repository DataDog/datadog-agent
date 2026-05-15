// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the defaultforwarder component.
//
// # V2 migration status: WIP
//
// This package is a placeholder for the V2 fx wiring of
// comp/forwarder/defaultforwarder. Once the implementation migration is
// complete (see comp/forwarder/defaultforwarder/impl), this package will
// provide the canonical Module() function using fxutil.ProvideComponentConstructor.
//
// Until then, callers should use the root-package Module() / NoopModule() /
// MockModule() functions.
package fx

import (
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
//
// TODO(migration): switch to fxutil.ProvideComponentConstructor once
// defaultforwarderimpl.NewComponent is fully implemented.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			defaultforwarderimpl.NewComponent,
		),
	)
}
