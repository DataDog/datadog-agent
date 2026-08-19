// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the PAR signing-key snapshot component.
package fx

import (
	signingkeysimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/signingkeys/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the component module.
func Module() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(signingkeysimpl.NewComponent))
}
