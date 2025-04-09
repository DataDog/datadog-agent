// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ipcimpl implements the IPC component.
package ipcimpl

import (
	"crypto/tls"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
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
}

// NewComponent creates a new ipc component
func NewComponent(reqs Requires) Provides {
	var initFunc func(model.Reader) error

	if reqs.Params.AllowWriteArtifacts {
		reqs.Log.Infof("Load or create IPC artifacts")
		initFunc = util.CreateAndSetAuthToken
	} else {
		reqs.Log.Infof("Load IPC artifacts")
		initFunc = util.SetAuthToken
	}

	if err := initFunc(reqs.Conf); err != nil {
		reqs.Log.Errorf("could not load IPC artifacts: %s", err)
		return Provides{
			Comp: option.None[ipc.Component](),
		}
	}

	return Provides{
		Comp: option.New[ipc.Component](&ipcComp{
			logger: reqs.Log,
			conf:   reqs.Conf,
		}),
	}
}

// Get returns the session token
func (ipc *ipcComp) Get() string {
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
