// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package rcclientimpl is a remote config client that can run within the agent to receive
// configurations.
package rcclientimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	agentTaskTimeout = 5 * time.Minute
)

type dependencies struct {
	fx.In

	Log log.Component
	Lc  fx.Lifecycle

	Params            rcclient.Params             `optional:"true"`
	Listeners         []types.RCListener          `group:"rCListener"`
	TaskListeners     []types.RCAgentTaskListener `group:"rCAgentTaskListener"`
	SettingsComponent settings.Component
	Config            configcomp.Component
	SysprobeConfig    option.Option[sysprobeconfig.Component]
	IPC               ipc.Component
}

type rcClient struct {
	client        *client.Client
	clientMRF     *client.Client
	listeners     []types.RCListener
	taskListeners []types.RCAgentTaskListener
	m             sync.Mutex
	settings      settings.Component
	config        configcomp.Component
	sysprobeConf  option.Option[sysprobeconfig.Component]
}

// NewRemoteConfigClient creates a new remote config client
func NewRemoteConfigClient(deps dependencies) (rcclient.Component, error) {
	if deps.Params.AgentName == "" {
		return nil, fmt.Errorf("agent name cannot be empty")
	}
	if deps.Params.AgentVersion == "" {
		return nil, fmt.Errorf("agent version cannot be empty")
	}

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(deps.Config)
	if err != nil {
		return nil, err
	}

	clientOptions := []func(*client.Options){
		client.WithAgent(deps.Params.AgentName, deps.Params.AgentVersion),
		client.WithPollInterval(5 * time.Second),
	}

	c, err := client.NewUnverifiedGRPCClient(
		ipcAddress,
		pkgconfigsetup.GetIPCPort(),
		deps.IPC.GetAuthToken(),
		deps.IPC.GetTLSClientConfig(),
		clientOptions...,
	)
	if err != nil {
		return nil, err
	}

	var clientMRF *client.Client
	if deps.Config.GetBool("multi_region_failover.enabled") {
		clientMRF, err = client.NewUnverifiedGRPCClient(
			ipcAddress,
			pkgconfigsetup.GetIPCPort(),
			deps.IPC.GetAuthToken(),
			deps.IPC.GetTLSClientConfig(),
			clientOptions...,
		)
		if err != nil {
			return nil, err
		}
	}

	rc := &rcClient{
		client:        c,
		clientMRF:     clientMRF,
		listeners:     deps.Listeners,
		taskListeners: deps.TaskListeners,
		settings:      deps.SettingsComponent,
		config:        deps.Config,
		sysprobeConf:  deps.SysprobeConfig,
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			if deps.Config.GetBool("remote_configuration.enabled") {
				return rc.start()
			}
			return nil
		},
		OnStop: func(_ context.Context) error {
			rc.client.Close()
			if rc.clientMRF != nil {
				rc.clientMRF.Close()
			}
			return nil
		},
	})

	return rc, nil
}

func (rc *rcClient) start() error {
	rc.m.Lock()
	defer rc.m.Unlock()

	// Start the client
	rc.client.Start()

	// Subscribe to listeners
	for _, listener := range rc.listeners {
		for product, callback := range listener {
			rc.client.Subscribe(string(product), callback)
		}
	}

	return nil
}

// SubscribeAgentTask subscribes the remote-config client to AGENT_TASK
func (rc *rcClient) SubscribeAgentTask() {
	// Implementation for agent task subscription
}

// Subscribe is the generic way to start listening to a specific product update
func (rc *rcClient) Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	rc.m.Lock()
	defer rc.m.Unlock()
	rc.client.Subscribe(string(product), fn)
}
