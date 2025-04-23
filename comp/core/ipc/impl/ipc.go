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
	reqs.Log.Infof("Load IPC artifacts")

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

// NewDebugOnlyComponent returns a new ipc component even if the auth artifacts are not found/initialized.
// This constructor covers cases where it is acceptable to not have initialized IPC component.
// This is typically the case for commands that MUST work no matter the coreAgent is running or not, if the auth artifacts are not found/initialized.
// A good example is the `flare` command, which should return a flare even if the IPC component is not initialized.
func NewDebugOnlyComponent(reqs Requires) Provides {
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
