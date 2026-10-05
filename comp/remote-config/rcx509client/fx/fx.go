// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the Fx module for the Remote Config x509 client.
package fx

import (
	uberfx "go.uber.org/fx"

	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	rcx509clientimpl "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the Fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		uberfx.Provide(rcx509clientimpl.NewClientFactory),
		fxutil.ProvideComponentConstructor(rcx509clientimpl.New),
		uberfx.Invoke(func(_ rcx509client.Component) {}),
	)
}
