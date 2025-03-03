// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package createandfetchimpl implements the creation and access to the auth_token used to communicate between Agent
// processes.
package createandfetchimpl

import (
	"crypto/tls"
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAuthToken),
		fxutil.ProvideOptional[authtoken.Component](),
	)
}

type authToken struct {
	logger log.Component
	conf   config.Component
}

var _ authtoken.Component = (*authToken)(nil)

type dependencies struct {
	fx.In

	Conf config.Component
	Log  log.Component
}

func newAuthToken(deps dependencies) (authtoken.Component, error) {
	if err := util.CreateAndSetAuthToken(deps.Conf); err != nil {
		deps.Log.Errorf("could not create auth_token: %s", err)
		return nil, err
	}

	return &authToken{
		logger: deps.Log,
		conf:   deps.Conf,
	}, nil
}

// Get returns the session token
func (at *authToken) Get() string {
	return util.GetAuthToken()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (at *authToken) GetTLSClientConfig() *tls.Config {
	return util.GetTLSClientConfig()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Client
func (at *authToken) GetTLSServerConfig() *tls.Config {
	return util.GetTLSServerConfig()
}

func (at *authToken) HTTPMiddleware(next http.Handler) http.Handler {
	return authtoken.NewHTTPMiddleware(at.logger.Warnf, at.Get())(next)
}

func (at *authToken) GetClient(_ ...authtoken.ClientOption) authtoken.SecureClient {
	return authtoken.NewClient(at.Get(), at.GetTLSClientConfig(), at.conf)
}
