// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package authtokenimpl implements the access to the auth_token used to communicate between Agent
// processes.
package authtokenimpl

import (
	"crypto/tls"
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/authtoken"
	"github.com/DataDog/datadog-agent/comp/core/authtoken/secureclient"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAuthToken),
		fx.Provide(newOptionalAuthToken),
	)
}

type authToken struct {
	logger log.Component
	conf   config.Component
}

var _ authtoken.Component = (*authToken)(nil)

type dependencies struct {
	fx.In

	Conf   config.Component
	Log    log.Component
	Params authtoken.Params
}

func newOptionalAuthToken(deps dependencies) option.Option[authtoken.Component] {
	var initFunc func(model.Reader) error

	if deps.Params.AllowWriteArtifacts {
		deps.Log.Infof("Load or create auth token")
		initFunc = util.CreateAndSetAuthToken
	} else {
		deps.Log.Infof("Load auth token")
		initFunc = util.SetAuthToken
	}

	if err := initFunc(deps.Conf); err != nil {
		deps.Log.Errorf("could not load auth artifact: %s", err)
		return option.None[authtoken.Component]()
	}

	return option.New[authtoken.Component](&authToken{
		logger: deps.Log,
		conf:   deps.Conf,
	})
}

type optionalDependencies struct {
	fx.In
	At  option.Option[authtoken.Component]
	Log log.Component
}

func newAuthToken(deps optionalDependencies) (authtoken.Component, error) {
	auth, ok := deps.At.Get()
	if !ok {
		return nil, deps.Log.Errorf("auth token not found")
	}
	return auth, nil
}

// Get returns the session token
func (at *authToken) Get() string {
	return util.GetAuthToken()
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (at *authToken) GetTLSClientConfig() *tls.Config {
	return util.GetTLSClientConfig()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (at *authToken) GetTLSServerConfig() *tls.Config {
	return util.GetTLSServerConfig()
}

func (at *authToken) HTTPMiddleware(next http.Handler) http.Handler {
	return authtoken.NewHTTPMiddleware(func(format string, params ...interface{}) { at.logger.Warnf(format, params...) }, at.Get())(next)
}

func (at *authToken) GetClient(_ ...authtoken.ClientOption) authtoken.SecureClient {
	return secureclient.NewClient(at.Get(), at.GetTLSClientConfig(), at.conf)
}
