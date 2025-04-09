// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package ipcimpl implements the IPC component.
package ipcimpl

import (
	"crypto/tls"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/ipc"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newIPC),
		fx.Provide(newOptionalIPC),
	)
}

type ipcComp struct {
	logger log.Component
	conf   config.Component
}

var _ ipc.Component = (*ipcComp)(nil)

type dependencies struct {
	fx.In

	Conf   config.Component
	Log    log.Component
	Params ipc.Params
}

func newOptionalIPC(deps dependencies) option.Option[ipc.Component] {
	var initFunc func(model.Reader) error

	if deps.Params.AllowWriteArtifacts {
		deps.Log.Infof("Load or create IPC artifacts")
		initFunc = util.CreateAndSetAuthToken
	} else {
		deps.Log.Infof("Load IPC artifacts")
		initFunc = util.SetAuthToken
	}

	if err := initFunc(deps.Conf); err != nil {
		deps.Log.Errorf("could not load IPC artifacts: %s", err)
		return option.None[ipc.Component]()
	}

	return option.New[ipc.Component](&ipcComp{
		logger: deps.Log,
		conf:   deps.Conf,
	})
}

type optionalIPCComp struct {
	fx.In
	At  option.Option[ipc.Component]
	Log log.Component
}

func newIPC(deps optionalIPCComp) (ipc.Component, error) {
	ipc, ok := deps.At.Get()
	if !ok {
		return nil, deps.Log.Errorf("ipc component has not been initialized")
	}
	return ipc, nil
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
