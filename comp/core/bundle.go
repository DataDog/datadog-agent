// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package core implements the "core" bundle, providing services common to all
// agent flavors and binaries.
//
// The constituent components serve as utilities and are mostly independent of
// one another.  Other components should depend on any components they need.
//
// This bundle does not depend on any other bundles.
package core

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx"
	delegatedauthnoopfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx-noop"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-runtimes

type bundleOptions struct {
	secretsModule       fx.Option
	delegatedAuthModule fx.Option
}

// Option changes some module implementations included in the bundle
type Option func(params *bundleOptions)

// Bundle defines the fx options for this bundle.
func Bundle(options ...Option) fxutil.BundleOptions {
	params := &bundleOptions{
		secretsModule:       secretsnoopfx.Module(),
		delegatedAuthModule: delegatedauthnoopfx.Module(),
	}
	for _, option := range options {
		option(params)
	}

	opts := []fx.Option{
		// As `config.Module` expects `config.Params` as a parameter, it is require to define how to get `config.Params` from `BundleParams`.
		fx.Provide(func(params BundleParams) config.Params { return params.ConfigParams }),
		config.Module(),
		fx.Provide(func(params BundleParams) log.Params { return params.LogParams }),
		logfx.Module(),
		fx.Provide(func(params BundleParams) sysprobeconfigimpl.Params { return params.SysprobeConfigParams }),
		sysprobeconfigimpl.Module(),
		telemetryimpl.Module(),
		pidimpl.Module(), // You must supply pidimpl.NewParams in order to use it
		params.secretsModule,
		params.delegatedAuthModule,
	}

	return fxutil.Bundle(
		opts...,
	)
}

// WithSecrets adds the secrets module to the bundle
func WithSecrets() Option {
	return func(params *bundleOptions) {
		params.secretsModule = secretsfx.Module()
	}
}

// WithDelegatedAuth adds the delegated auth module to the bundle
func WithDelegatedAuth() Option {
	return func(params *bundleOptions) {
		params.delegatedAuthModule = delegatedauthfx.Module()
	}
}
