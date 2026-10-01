// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx wires the cluster ID resolver into Fx.
package fx

import (
	clusteridresolverimpl "github.com/DataDog/datadog-agent/comp/core/clusteridresolver/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides the cluster ID resolver.
func Module() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(clusteridresolverimpl.NewComponent))
}
