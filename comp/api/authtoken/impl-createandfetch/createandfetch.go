// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package createandfetchimpl implements the authtoken component interface
// It create or fetch the auth_token depending if it is already existing in the file system
package createandfetchimpl

import (
	"crypto/tls"

	authtoken "github.com/DataDog/datadog-agent/comp/api/authtoken/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
)

type authToken struct{}

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
func NewComponent(reqs Requires) (Provides, error) {
	if err := util.CreateAndSetAuthToken(reqs.Conf); err != nil {
		reqs.Log.Error("could not create auth_token: %s", err)
		return Provides{}, err
	}

	return Provides{Comp: &authToken{}}, nil
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
