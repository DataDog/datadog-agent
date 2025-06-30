// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines the types used by the autodiscovery providers
package types

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// ErrorMsgSet contains a list of unique configuration errors for a provider
type ErrorMsgSet map[string]struct{}

// ConfigProviderFactory is any function capable to create a ConfigProvider instance
type ConfigProviderFactory func(providerConfig *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, telemetryStore *telemetry.Store) (ConfigProvider, error)

// ConfigProvider represents a source of `integration.Config` values
// that can either be applied immediately or resolved for a service and
// applied.
//
// These Config values may come from files on disk, databases, environment variables,
// container labels, etc.
//
// Any type implementing the interface will take care of any dependency
// or data needed to access the resource providing the configuration.
type ConfigProvider interface {
	// String returns the name of the provider.  All Config instances produced
	// by this provider will have this value in their Provider field.
	String() string

	// GetConfigErrors returns a map of errors that occurred on the last Collect
	// call, indexed by a description of the resource that generated the error.
	// The result is displayed in diagnostic tools such as `agent status`.
	GetConfigErrors() map[string]ErrorMsgSet
}

// CollectingConfigProvider is an interface used together with ConfigProvider.
// ConfigProviders that are NOT able to use streaming, and therefore need external reconciliation, should implement it.
type CollectingConfigProvider interface {
	// Collect is responsible of populating a list of Config instances by
	// retrieving configuration patterns from external resources.
	Collect(context.Context) ([]integration.Config, error)

	// IsUpToDate determines whether the information returned from the last
	// call to Collect is still correct.  If not, Collect will be called again.
	IsUpToDate(context.Context) (bool, error)
}

// StreamingConfigProvider is an interface used together with ConfigProvider.
// ConfigProviders that are able to use streaming should implement it, and the
// config poller will use Stream instead of Collect to collect config changes.
type StreamingConfigProvider interface {
	// Stream starts the streaming config provider until the provided
	// context is cancelled. Config changes are sent on the return channel.
	Stream(context.Context) <-chan integration.ConfigChanges
}
