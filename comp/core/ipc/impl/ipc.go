// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ipcimpl implements the IPC component.
package ipcimpl

import (
	"crypto/tls"
	"net/http"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
)

// Requires defines the dependencies for the ipc component
type Requires struct {
	Conf config.Component
	Log  log.Component
}

// Provides defines the output of the ipc component
type Provides struct {
	Comp       ipc.Component
	HTTPClient ipc.HTTPClient
}

type ipcComp struct {
	logger log.Component
	conf   config.Component
	client ipc.HTTPClient
}

// NewReadOnlyComponent creates a new ipc component by trying to read the auth artifacts on filesystem.
// If the auth artifacts are not found, it will return an error.
func NewReadOnlyComponent(reqs Requires) (Provides, error) {
	reqs.Log.Debug("Loading IPC artifacts")
	if err := util.SetAuthToken(reqs.Conf); err != nil {
		return Provides{}, err
	}

	httpClient := ipchttp.NewClient(util.GetAuthToken(), util.GetTLSClientConfig(), reqs.Conf)

	return Provides{
		Comp: &ipcComp{
			logger: reqs.Log,
			conf:   reqs.Conf,
			client: httpClient,
		},
		HTTPClient: httpClient,
	}, nil
}

// NewReadWriteComponent creates a new ipc component by trying to read the auth artifacts on filesystem,
// and if they are not found, it will create them.
func NewReadWriteComponent(reqs Requires) (Provides, error) {
	reqs.Log.Debug("Loading or creating IPC artifacts")
	if err := util.CreateAndSetAuthToken(reqs.Conf); err != nil {
		return Provides{}, err
	}

	httpClient := ipchttp.NewClient(util.GetAuthToken(), util.GetTLSClientConfig(), reqs.Conf)

	return Provides{
		Comp: &ipcComp{
			logger: reqs.Log,
			conf:   reqs.Conf,
			client: httpClient,
		},
		HTTPClient: httpClient,
	}, nil
}

// NewInsecureComponent creates an IPC component instance suitable for specific commands
// (like 'flare' or 'diagnose') that must function even when the main Agent isn't running
// or IPC artifacts (like auth tokens) are missing or invalid.
//
// This constructor *always* succeeds, unlike NewReadWriteComponent or NewReadOnlyComponent
// which might fail if artifacts are absent or incorrect.
// However, the resulting IPC component instance might be non-functional or only partially
// functional, potentially leading to failures later, such as rejected connections during
// the IPC handshake if communication with the core Agent is attempted.
//
// WARNING: This constructor is intended *only* for edge cases like diagnostics and flare generation.
// Using it in standard agent processes or commands that rely on stable IPC communication
// will likely lead to unexpected runtime errors or security issues.
func NewInsecureComponent(reqs Requires) Provides {
	reqs.Log.Debug("Loading IPC artifacts (insecure)")
	provides, err := NewReadOnlyComponent(reqs)
	if err == nil {
		return provides
	}

	reqs.Log.Warnf("Failed to create ipc component: %v", err)

	httpClient := ipchttp.NewClient("", &tls.Config{}, reqs.Conf)

	return Provides{
		Comp: &ipcComp{
			logger: reqs.Log,
			conf:   reqs.Conf,
			client: httpClient,
		},
		HTTPClient: httpClient,
	}
}

// GetAuthToken returns the session token
func (ipc *ipcComp) GetAuthToken() string {
	return util.GetAuthToken()
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (ipc *ipcComp) GetTLSClientConfig() *tls.Config {
	return util.GetTLSClientConfig()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (ipc *ipcComp) GetTLSServerConfig() *tls.Config {
	return util.GetTLSServerConfig()
}

func (ipc *ipcComp) HTTPMiddleware(next http.Handler) http.Handler {
	return ipchttp.NewHTTPMiddleware(func(format string, params ...interface{}) {
		ipc.logger.Errorf(format, params...)
	}, ipc.GetAuthToken())(next)
}

func (ipc *ipcComp) GetClient() ipc.HTTPClient {
	return ipc.client
}
