// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tagger

import (
	"crypto/tls"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Option is a function that modifies the RemoteParams
type Option = func(*RemoteParams)

// WithRemoteTarget sets the RemoteTarget function
func WithRemoteTarget(remoteTarget func(config.Component) (string, error)) Option {
	return func(params *RemoteParams) {
		params.RemoteTarget = remoteTarget
	}
}

// WithRemoteFilter sets the RemoteFilter function
func WithRemoteFilter(filter *types.Filter) Option {
	return func(params *RemoteParams) {
		params.RemoteFilter = filter
	}
}

// WithOverrideTLSConfigGetter sets the OverrideTLSConfigGetter function
func WithOverrideTLSConfigGetter(getter func() (*tls.Config, error)) Option {
	return func(params *RemoteParams) {
		params.OverrideTLSConfigGetter = getter
	}
}

// WithOverrideAuthTokenGetter sets the OverrideAuthTokenGetter function
func WithOverrideAuthTokenGetter(getter func(pkgconfigmodel.Reader) (string, error)) Option {
	return func(params *RemoteParams) {
		params.OverrideAuthTokenGetter = getter
	}
}

// NewRemoteParams creates a new RemoteParams with the default values
func NewRemoteParams(opts ...Option) RemoteParams {
	params := RemoteParams{
		RemoteTarget: func(c config.Component) (string, error) {
			return fmt.Sprintf(":%v", c.GetInt("cmd_port")), nil
		},
		RemoteFilter: types.NewMatchAllFilter(),
	}
	for _, opt := range opts {
		opt(&params)
	}
	return params
}

// RemoteParams provides remote tagger parameters
type RemoteParams struct {
	// RemoteFilter is the filter to apply to the remote tagger when streaming tag events
	RemoteFilter *types.Filter
	// RemoteTarget function return the target in which the remote tagger will connect
	// If it returns an error we stop the application
	RemoteTarget func(config.Component) (string, error)

	// OverrideTLSConfigGetter allows to override the TLS configuration used by the remote tagger
	// This should be used only for Cluster Agent x CLC communication
	OverrideTLSConfigGetter func() (*tls.Config, error)

	// OverrideAuthTokenGetter allows to override the auth token used by the remote tagger
	// This should be used only for Cluster Agent x CLC communication
	OverrideAuthTokenGetter func(pkgconfigmodel.Reader) (string, error)
}

// DualParams provides dual tagger parameters
type DualParams struct {
	// UseRemote is a function to determine if the remote tagger should be used
	UseRemote func(config.Component) bool
}

// OptionalRemoteParams provides the optional remote tagger parameters
type OptionalRemoteParams struct {
	// Disable opts out of the remote tagger in favor of the noop tagger
	Disable func() bool
}
