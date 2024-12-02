// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fetchonlyimpl implements the access to the auth_token used to communicate between Agent
// processes but does not create it.
package fetchonlyimpl

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"go.uber.org/fx"

	"github.com/cenkalti/backoff"

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

		// Force the component to be constructed to run the periodic auth_token fetch.
		// This approach is used because the coreAgent is currently responsible for auth_token generation,
		// but there is no defined startup sequence between Agent processes.
		// Some functions depend on the auth_token being initialized to work correctly.
		// Running a background routine that periodically fetches the auth_token until it is retrieved
		// helps reduce the number of cases where it is used uninitialized.
		fx.Invoke(func(_ authtoken.Component) {}),
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
	Lc   fx.Lifecycle
}

func newAuthToken(deps dependencies) authtoken.Component {

	// Create a ticker that triggers every 5 seconds.
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = 60 * time.Second
	expBackoff.MaxElapsedTime = 5 * time.Minute
	expBackoff.Reset()
	ticker := backoff.NewTicker(expBackoff)

	// Create a channel to signal the goroutine to stop.
	stopChan := make(chan struct{})

	comp := authToken{
		log:  deps.Log,
		conf: deps.Conf,
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			deps.Log.Debugf("starting auth_token periodic fetch until getting it")
			go func() {
				for {
					select {
					case <-ticker.C:
						err := util.SetAuthToken(deps.Conf)
						if err == nil {
							deps.Log.Infof("auth_token have been initialized")
							comp.tokenLoaded = true
							return
						}
						deps.Log.Infof("auth_token not initialized yet: %v", err.Error())
					case <-stopChan:
						return
					}
				}
			}()
			return nil
		},
		OnStop: func(_ context.Context) error {
			ticker.Stop()
			close(stopChan)
			return nil
		},
	})

	return &comp
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
func (at *authToken) Get() string {
	if err := at.setToken(); err != nil {
		at.log.Debugf(err.Error())
		return ""
	}

	return util.GetAuthToken()
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (at *authToken) GetTLSClientConfig() *tls.Config {
	if err := at.setToken(); err != nil {
		at.log.Debugf(err.Error())
		return nil
	}

	return util.GetTLSClientConfig()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (at *authToken) GetTLSServerConfig() *tls.Config {
	if err := at.setToken(); err != nil {
		at.log.Debugf(err.Error())
		return nil
	}

	return util.GetTLSServerConfig()
}
