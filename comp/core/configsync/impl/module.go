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

// Requires defines the dependencies for the configsync component.
type Requires struct {
	Lc compdef.Lifecycle

	Config    config.Component
	Log       log.Component
	IPCClient ipc.HTTPClient
	Params    Params
}

// Provides defines the output of the configsync component.
type Provides struct {
	Comp configsync.Component
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

// NewComponent checks if the component was enabled as per the config and returns an enabled/disabled configsync
func NewComponent(reqs Requires) (Provides, error) {
	configRefreshIntervalSec := reqs.Config.GetInt("agent_ipc.config_refresh_interval")
	if configRefreshIntervalSec <= 0 {
		reqs.Log.Infof("configsync disabled: agent_ipc.config_refresh_interval invalid: %d)", configRefreshIntervalSec)
		return Provides{Comp: configSync{}}, nil
	}
	configRefreshInterval := time.Duration(configRefreshIntervalSec) * time.Second

	var compURL *url.URL
	if reqs.Config.GetBool("agent_ipc.use_socket") {
		compURL = &url.URL{
			Scheme: "https+unix",
			Host:   "localhost:",
			Path:   "/config/v1/",
		}
	} else {
		host := reqs.Config.GetString("agent_ipc.host")
		port := reqs.Config.GetInt("agent_ipc.port")
		if port <= 0 {
			reqs.Log.Infof("configsync disabled: agent_ipc.port invalid: %d)", port)
			return Provides{Comp: configSync{}}, nil
		}

		compURL = &url.URL{
			Scheme: "https",
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
			Path:   "/config/v1/",
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cs := configSync{
		Config:  reqs.Config,
		Log:     reqs.Log,
		url:     compURL,
		client:  reqs.IPCClient,
		ctx:     ctx,
		timeout: reqs.Params.Timeout,
		enabled: true,
	}

	if reqs.Params.OnInitSync {
		reqs.Log.Infof("triggering configsync on init (will retry for %s)", reqs.Params.OnInitSyncTimeout)
		deadline := time.Now().Add(reqs.Params.OnInitSyncTimeout)
		for {
			if err := cs.updater(); err == nil {
				break
			}
			if time.Now().After(deadline) {
				cancel()
				return Provides{}, reqs.Log.Errorf("failed to sync config at startup, is the core agent listening on '%s' ?", compURL.String())
			}
			time.Sleep(2 * time.Second)
		}
		reqs.Log.Infof("triggering configsync on init succeeded")
	}

	// start and stop the routine in lifecycle hooks
	reqs.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			go cs.runWithInterval(configRefreshInterval)
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return nil
		},
	})

	reqs.Log.Infof("configsync enabled (agent_ipc '%s' | agent_ipc.config_refresh_interval: %d)", compURL.Host, configRefreshIntervalSec)
	return Provides{Comp: cs}, nil
}
