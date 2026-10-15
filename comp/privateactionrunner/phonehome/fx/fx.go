// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx wires the PAR phone-home component into core.
package fx

import (
	phonehome "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	phonehomeimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module registers the component and its optional status dependency.
func Module() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(phonehomeimpl.NewComponent), fxutil.ProvideOptional[phonehome.Component]())
}
