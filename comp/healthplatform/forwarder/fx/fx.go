// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the health platform forwarder component.
package fx

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	forwarderimpl "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Requires defines the dependencies for the forwarder fx module.
type Requires struct {
	Log      log.Component
	Config   config.Component
	Hostname hostnameinterface.Component
}

// Module defines the fx options for the forwarder component.
// The issue provider is wired separately via SetProvider() in the lifecycle start hook
// to avoid a circular dependency with the core health platform component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newForwarder),
	)
}

func newForwarder(reqs Requires) forwarderdef.Component {
	hostname, err := reqs.Hostname.Get(context.Background())
	if err != nil {
		reqs.Log.Warn("Health platform forwarder: failed to get hostname, will use empty string: " + err.Error())
		hostname = ""
	}
	return forwarderimpl.New(reqs.Log, reqs.Config, hostname)
}
