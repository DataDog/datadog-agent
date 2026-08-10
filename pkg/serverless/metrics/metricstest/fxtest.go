// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package metricstest provides test helpers for constructing the Fx-managed
// forwarder, demultiplexer and DogStatsD server bundle used by serverless-init.
// Tests build a ServerlessMetricAgent on top of the returned components.
package metricstest

import (
	"testing"

	"go.uber.org/fx"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-noop"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	filterlistfx "github.com/DataDog/datadog-agent/comp/filterlist/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformfx "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/fx"
	eventplatformreceiverimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/impl"
	orchestrator "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	orchestratorfx "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/fx"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Deps is the set of components the serverless-init metric agent depends on.
// It mirrors the bundle wired up in cmd/serverless-init/main.go.
type Deps struct {
	fx.In

	DogstatsdServer dogstatsdServer.Component
	Demultiplexer   demultiplexer.Component
	Demux           aggregator.Demultiplexer
	Forwarder       defaultforwarder.Component
}

// bundleOptions returns the shared module graph used by both New and StartBundle.
// It mirrors the bundle wired up in cmd/serverless-init/main.go, with the
// forwarder supplied separately so callers can inject a counting/fake forwarder.
func bundleOptions(t *testing.T, taggerComp tagger.Component, forwarderOpts fx.Option) fx.Option {
	return fx.Options(
		fx.Supply(configcomp.NewParams("")),
		configcomp.Module(),
		fx.Provide(func() logdef.Component { return logmock.New(t) }),
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		fx.Provide(func() delegatedauth.Component { return delegatedauthmock.New(t) }),
		fx.Provide(func() tagger.Component { return taggerComp }),
		nooptelemetry.Module(),
		workloadmetafx.Module(workloadmeta.NewParams()),
		orchestratorfx.Module(orchestrator.NewNoopParams()),
		eventplatformfx.Module(eventplatform.NewDisabledParams()),
		eventplatformreceiverimpl.Module(),
		haagentfx.Module(),
		metricscompressionfx.Module(),
		logscompressionfx.Module(),
		filterlistfx.Module(),
		hostnameimpl.MockModule(),
		forwarderOpts,
		demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(
			demultiplexerimpl.WithContinueOnMissingHostname(),
		)),
		dogstatsd.Bundle(dogstatsdServer.Params{Serverless: true}),
	)
}

// New constructs the Fx-managed forwarder, demultiplexer and DogStatsD server
// using the same module graph as cmd/serverless-init/main.go. The provided
// tagger component is injected into the graph.
func New(t *testing.T, taggerComp tagger.Component) Deps {
	t.Helper()
	return fxutil.Test[Deps](t, bundleOptions(t, taggerComp, forwarder.Bundle(defaultforwarder.NewParams())))
}

// StartBundle constructs and starts the same Fx graph as New, but injects the
// caller-supplied forwarder (typically a counting/fake forwarder) in place of
// the real one. It returns the started app and its Deps; the caller drives the
// shutdown cascade by invoking app.Stop, which fires the OnStop hooks in
// reverse construction order — dsdServer.stop -> demux.Stop() ->
// forwarder.Stop — exactly as cmd/serverless-init does on shutdown.
//
// The caller is responsible for calling app.Stop; it is not registered with
// t.Cleanup so the test controls exactly when the cascade runs.
func StartBundle(t *testing.T, taggerComp tagger.Component, fwd defaultforwarder.Component) (*fx.App, Deps) {
	t.Helper()
	forwarderOpts := fx.Provide(func() defaultforwarder.Component { return fwd })
	app, deps, err := fxutil.TestApp[Deps](bundleOptions(t, taggerComp, forwarderOpts))
	if err != nil {
		t.Fatalf("failed to start metricstest bundle: %v", err)
	}
	return app, deps
}
