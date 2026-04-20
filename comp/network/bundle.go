// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package network implements the "network" bundle, providing network monitoring components.
package network

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	localtraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-local"
	remotetraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
	networktracerimpl "github.com/DataDog/datadog-agent/comp/networktracer/fx"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: cloud-network-monitoring

type bundleOptions struct {
	tracerouteModule    fx.Option
	networkTracerModule fx.Option
}

// Option modifies the set of modules included in the bundle.
type Option func(*bundleOptions)

// WithRemoteTraceroute selects the remote (gRPC-backed) traceroute implementation.
// This is the default when no traceroute option is provided.
func WithRemoteTraceroute() Option {
	return func(o *bundleOptions) {
		o.tracerouteModule = remotetraceroute.Module()
	}
}

// WithLocalTraceroute selects the local (in-process) traceroute implementation.
// Use this in binaries that run system-probe in the same process (i.e. system-probe itself).
func WithLocalTraceroute() Option {
	return func(o *bundleOptions) {
		o.tracerouteModule = localtraceroute.Module()
	}
}

// WithNetworkTracer includes the networktracer component, which owns the
// pkg/network/tracer lifecycle. Only system-probe should use this option.
func WithNetworkTracer() Option {
	return func(o *bundleOptions) {
		o.networkTracerModule = networktracerimpl.Module()
	}
}

// Bundle defines the fx options for the network monitoring bundle.
//
// By default the bundle wires:
//   - rdnsquerier (reverse DNS enrichment)
//   - npcollector (network path collector)
//   - traceroute via the remote implementation
//
// Pass option functions to customise the implementation variants:
//
//	network.Bundle(network.WithLocalTraceroute(), network.WithNetworkTracer())
func Bundle(options ...Option) fxutil.BundleOptions {
	opts := &bundleOptions{
		tracerouteModule:    remotetraceroute.Module(),
		networkTracerModule: fx.Options(), // no-op by default
	}
	for _, o := range options {
		o(opts)
	}

	return fxutil.Bundle(
		rdnsquerierfx.Module(),
		opts.tracerouteModule,
		npcollectorimpl.Module(),
		opts.networkTracerModule,
	)
}
