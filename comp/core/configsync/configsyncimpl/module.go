// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configsyncimpl implements synchronizing the configuration using the core agent config API
package configsyncimpl

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	Config     config.Component
	Log        log.Component
	IPCClient  ipc.HTTPClient
	SyncParams Params
}

// Module defines the fx options for this component.
func Module(params Params) fxutil.Module {
	return fxutil.Component(
		fx.Provide(newComponent),
		fx.Supply(params),

		// configSync is a component with no public method, therefore nobody depends on it and FX only instantiates
		// components when they're needed. Adding a dummy function that takes our Component as a parameter force
		// the instantiation of configsync. This means that simply using 'configsync.Module()' will run our
		// component (which is the expected behavior).
		//
		// This prevent silent corner case where including 'configsync' in the main function would not actually
		// instantiate it. This also remove the need for every main using configsync to add the line bellow.
		fx.Invoke(func(_ configsync.Component) {}),
	)
}

type configSync struct {
	Config config.Component
	Log    log.Component

	url       *url.URL
	client    ipc.HTTPClient
	connected bool
	ctx       context.Context
	timeout   time.Duration
	enabled   bool
}

// newComponent checks if the component was enabled as per the config and return a enable/disabled configsync
func newComponent(deps dependencies) (configsync.Component, error) {
	agentIPCPort := deps.Config.GetInt("agent_ipc.port")
	configRefreshIntervalSec := deps.Config.GetInt("agent_ipc.config_refresh_interval")

	if agentIPCPort <= 0 || configRefreshIntervalSec <= 0 {
		deps.Log.Infof("configsync disabled (agent_ipc.port: %d | agent_ipc.config_refresh_interval: %d)", agentIPCPort, configRefreshIntervalSec)
		return configSync{}, nil
	}

	deps.Log.Infof("configsync enabled (agent_ipc '%s:%d' | agent_ipc.config_refresh_interval: %d)", deps.Config.GetString("agent_ipc.host"), agentIPCPort, configRefreshIntervalSec)
	return newConfigSync(deps, agentIPCPort, configRefreshIntervalSec)
}

// newConfigSync creates a new configSync component.
// agentIPCPort and configRefreshIntervalSec must be strictly positive.
func newConfigSync(deps dependencies, agentIPCPort int, configRefreshIntervalSec int) (configsync.Component, error) {
	agentIPCHost := deps.Config.GetString("agent_ipc.host")

	url := &url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(agentIPCHost, strconv.Itoa(agentIPCPort)),
		Path:   "/config/v1",
	}

	ctx, cancel := context.WithCancel(context.Background())
	configRefreshInterval := time.Duration(configRefreshIntervalSec) * time.Second

	configSync := configSync{
		Config:  deps.Config,
		Log:     deps.Log,
		url:     url,
		client:  deps.IPCClient,
		ctx:     ctx,
		timeout: deps.SyncParams.Timeout,
		enabled: true,
	}

	if deps.SyncParams.OnInitSync {
		deps.Log.Infof("triggering configsync on init (will retry for %s)", deps.SyncParams.OnInitSyncTimeout)
		deadline := time.Now().Add(deps.SyncParams.OnInitSyncTimeout)
		for {
			if err := configSync.updater(); err == nil {
				break
			}
			if time.Now().After(deadline) {
				cancel()
				return nil, deps.Log.Errorf("failed to sync config at startup, is the core agent listening on '%s' ?", url.String())
			}
			time.Sleep(2 * time.Second)
		}
		deps.Log.Infof("triggering configsync on init succeeded")
	}

	// start and stop the routine in fx hooks
	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go configSync.runWithInterval(configRefreshInterval)
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return nil
		},
	})

	return configSync, nil
}
