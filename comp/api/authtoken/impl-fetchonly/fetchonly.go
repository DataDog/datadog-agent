// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fetchonlyimpl implements the authtoken component interface
// It fetch the auth_token from the file system
package fetchonlyimpl

import (
	"crypto/tls"
	"fmt"

	authtoken "github.com/DataDog/datadog-agent/comp/api/authtoken/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
)

type authToken struct {
	log         log.Component
	conf        config.Component
	tokenLoaded bool
}

var _ authtoken.Component = (*authToken)(nil)

// Requires defines the dependencies for the authtoken component
type Requires struct {
	Conf config.Component
	Log  log.Component
}

// Provides defines the output of the authtoken component
type Provides struct {
	Comp authtoken.Component
}

// NewComponent creates a new authtoken component
func NewComponent(reqs Requires) Provides {
	return Provides{Comp: &authToken{
		log:  reqs.Log,
		conf: reqs.Conf,
	}}
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
		at.log.Debugf("%s", err.Error())
		return ""
	}

	return util.GetAuthToken()
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (at *authToken) GetTLSClientConfig() *tls.Config {
	if err := at.setToken(); err != nil {
		at.log.Debugf("%s", err.Error())
		return nil
	}

	return util.GetTLSClientConfig()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (at *authToken) GetTLSServerConfig() *tls.Config {
	if err := at.setToken(); err != nil {
		at.log.Debugf("%s", err.Error())
		return nil
	}

	return util.GetTLSServerConfig()
}
