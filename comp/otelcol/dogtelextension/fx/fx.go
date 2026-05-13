// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the FX module for dogtelextension
package fx

import (
	dogtelextension "github.com/DataDog/datadog-agent/comp/otelcol/dogtelextension/def"
	dogtelextensionimpl "github.com/DataDog/datadog-agent/comp/otelcol/dogtelextension/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
//
// Note: This FX module is not currently used, as the dogtelextension is
// instantiated directly by the OTel collector framework, not via FX.
// The extension factory (NewFactoryForAgent) receives FX-injected components
// as parameters instead.
//
// This module is provided for potential future use if the architecture changes.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(dogtelextensionimpl.NewExtension),
		fxutil.ProvideOptional[dogtelextension.Component](),
	)
}
