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

	"github.com/DataDog/datadog-agent/comp/core/config"
	configsync "github.com/DataDog/datadog-agent/comp/core/configsync/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies of the configsync component.
type Requires struct {
	compdef.In
	Lc compdef.Lifecycle

	Config     config.Component
	Log        log.Component
	IPCClient  ipc.HTTPClient
	SyncParams configsync.Params
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

// NewComponent creates a new configsync component.
func NewComponent(deps Requires) (configsync.Component, error) {
	configRefreshIntervalSec := deps.Config.GetInt("agent_ipc.config_refresh_interval")
	if configRefreshIntervalSec <= 0 {
		deps.Log.Infof("configsync disabled: agent_ipc.config_refresh_interval invalid: %d)", configRefreshIntervalSec)
		return configSync{}, nil
	}
	configRefreshInterval := time.Duration(configRefreshIntervalSec) * time.Second

	var compURL *url.URL
	if deps.Config.GetBool("agent_ipc.use_socket") {
		compURL = &url.URL{
			Scheme: "https+unix", // +unix gets trimmed by the ipc client and indicates to use agent_ipc.socket_path in the ipc client
			Host:   "localhost:", // this is required for using the mTLS cert validating the hostname in the request
			Path:   "/config/v1/",
		}
	} else {
		host := deps.Config.GetString("agent_ipc.host")
		port := deps.Config.GetInt("agent_ipc.port")
		if port <= 0 {
			deps.Log.Infof("configsync disabled: agent_ipc.port invalid: %d)", port)
			return configSync{}, nil
		}

		compURL = &url.URL{
			Scheme: "https",
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
			Path:   "/config/v1/",
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	configSync := configSync{
		Config:  deps.Config,
		Log:     deps.Log,
		url:     compURL,
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
				return nil, deps.Log.Errorf("failed to sync config at startup, is the core agent listening on '%s' ?", compURL.String())
			}
			time.Sleep(2 * time.Second)
		}
		deps.Log.Infof("triggering configsync on init succeeded")
	}

	// start and stop the routine in fx hooks
	deps.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			go configSync.runWithInterval(configRefreshInterval)
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return nil
		},
	})

	deps.Log.Infof("configsync enabled (agent_ipc '%s' | agent_ipc.config_refresh_interval: %d)", compURL.Host, configRefreshIntervalSec)
	return configSync, nil
}
