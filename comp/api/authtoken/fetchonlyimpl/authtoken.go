// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fetchonlyimpl implements the access to the auth_token used to communicate between Agent
// processes but does not create it.
package fetchonlyimpl

import (
	"crypto/tls"
	"fmt"

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
	log         log.Component
	conf        config.Component
	tokenLoaded bool
}

var _ authtoken.Component = (*authToken)(nil)

type dependencies struct {
	fx.In

	Log  log.Component
	Conf config.Component
}

func newAuthToken(deps dependencies) authtoken.Component {
	return &authToken{
		log:  deps.Log,
		conf: deps.Conf,
	}
}

func (at *authToken) setToken() error {
	if !at.tokenLoaded {
		// We try to load the auth_token until we succeed since it might be created at some point by another
		// process.
		if err := util.SetAuthToken(at.conf); err != nil {
			return fmt.Errorf("could not load auth_token: %s", err)
		}
		at.tokenLoaded = true
	}
	return nil
}

// Get returns the session token
func (at *authToken) Get() (string, error) {
	if err := at.setToken(); err != nil {
		return "", err
	}

	return util.GetAuthToken(), nil
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (at *authToken) GetTLSClientConfig() *tls.Config {
	if err := at.setToken(); err != nil {
		at.log.Debugf("%s", err.Error())
	}

	return util.GetTLSClientConfig()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (at *authToken) GetTLSServerConfig() *tls.Config {
	if err := at.setToken(); err != nil {
		at.log.Debugf("%s", err.Error())
	}

	return util.GetTLSServerConfig()
}
