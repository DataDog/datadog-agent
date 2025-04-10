// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ipcimpl implements the ipc component interface
package ipcimpl

import (
	"crypto/tls"
	"net/http"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/http"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Requires defines the dependencies for the ipc component
type Requires struct {
	Conf   config.Component
	Log    log.Component
	Params ipc.Params
}

// Provides defines the output of the ipc component
type Provides struct {
	Comp option.Option[ipc.Component]
}

type ipcComp struct {
	logger log.Component
	conf   config.Component
	client ipc.HTTPClient
}

// NewComponent creates a new ipc component
func NewComponent(reqs Requires) Provides {
	var initFunc func(model.Reader) error
	var comp option.Option[ipc.Component]

	if reqs.Params.AllowWriteArtifacts {
		reqs.Log.Infof("Load or create IPC artifacts")
		initFunc = util.CreateAndSetAuthToken
	} else {
		reqs.Log.Infof("Load IPC artifacts")
		initFunc = util.SetAuthToken
	}

	if err := initFunc(reqs.Conf); err != nil {
		reqs.Log.Errorf("could not load IPC artifacts: %s", err)
		comp = option.None[ipc.Component]()
	} else {
		comp = option.New[ipc.Component](&ipcComp{
			logger: reqs.Log,
			conf:   reqs.Conf,
			client: ipchttp.NewClient(util.GetAuthToken(), util.GetTLSClientConfig(), reqs.Conf),
		})
	}

	return Provides{
		Comp: comp,
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
	return ipchttp.NewHTTPMiddleware(ipc.logger.Infof, ipc.GetAuthToken())(next)
}

func (ipc *ipcComp) GetClient() ipc.HTTPClient {
	return ipc.client
}
