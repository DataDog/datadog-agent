// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"context"
	"errors"
	"sync"

	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries"
)

const (
	executeEnabledSetting = "remote_queries.tunnel.execute_enabled"
	tunnelOrgIDSetting    = "remote_queries.tunnel.org_id"
)

// ConnectionTracker holds the RC connection ID that the connection reporter
// last observed from the handshake. It is empty before the first handshake and
// after the client is closed.
type ConnectionTracker struct {
	mu sync.RWMutex
	id string
}

// NewConnectionTracker returns an empty tracker.
func NewConnectionTracker() *ConnectionTracker { return &ConnectionTracker{} }

// set records the current connection ID. It is safe on a nil tracker.
func (t *ConnectionTracker) set(id string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.id = id
	t.mu.Unlock()
}

// Current returns the current connection ID, or "" when there is none.
func (t *ConnectionTracker) Current() string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.id
}

// tunnelAuthority binds a dispatch to the operator-configured org and to the
// Agent's current connection ID, as observed from the handshake. It is NOT an
// always-allow authority: it fails closed when either is unknown. It does not
// authenticate the caller; Remote Config authorizes who may call Execute, and
// this only lets the Tunnel check that the request names this Agent's live
// connection and configured org.
type tunnelAuthority struct {
	config     cfgcomp.Component
	connection func() string
}

func (a tunnelAuthority) Authorize(context.Context, uint64) (remotequeries.TunnelTrust, error) {
	orgID := a.config.GetInt64(tunnelOrgIDSetting)
	if orgID <= 0 {
		return remotequeries.TunnelTrust{}, errors.New("remote queries tunnel org ID is not configured")
	}
	id := a.connection()
	if id == "" {
		return remotequeries.TunnelTrust{}, errors.New("remote config connection ID is not known")
	}
	return remotequeries.TunnelTrust{OrgID: orgID, ConnectionID: id}, nil
}

// ExecuteHandlerDependencies are the dependencies of NewExecuteHandler.
type ExecuteHandlerDependencies struct {
	compdef.In

	Config      cfgcomp.Component
	Logger      log.Component
	Connections *ConnectionTracker
	// Collector is absent in Agent flavors that load no checks.
	Collector collector.Component `optional:"true"`
}

// ExecuteHandlerProvides is the output of NewExecuteHandler. Handler is nil
// when the Execute URI must stay unregistered.
type ExecuteHandlerProvides struct {
	compdef.Out

	Handler rcx509client.ExecuteHandler
}

// NewExecuteHandler provides the Remote Queries Execute handler, or a nil one
// when remote_queries.tunnel.execute_enabled is off (the default) or its
// preconditions are not met, in which case the URI stays unregistered. It must
// be registered with fxutil.ProvideComponentConstructor: a plain fx.Provide
// would not understand the compdef.In struct, and the optional consumer would
// silently receive nothing.
func NewExecuteHandler(deps ExecuteHandlerDependencies) ExecuteHandlerProvides {
	return ExecuteHandlerProvides{Handler: buildExecuteHandler(deps)}
}

func buildExecuteHandler(deps ExecuteHandlerDependencies) rcx509client.ExecuteHandler {
	if !deps.Config.GetBool(executeEnabledSetting) {
		return nil
	}
	if deps.Config.GetInt64(tunnelOrgIDSetting) <= 0 {
		deps.Logger.Errorf("remote queries tunnel execute handler not registered: %s must be a positive org ID", tunnelOrgIDSetting)
		return nil
	}
	if !deps.Config.GetBool(reportConnectionIDSetting) {
		deps.Logger.Errorf("remote queries tunnel execute handler not registered: %s must be enabled so the Agent knows its connection ID", reportConnectionIDSetting)
		return nil
	}
	if deps.Collector == nil || deps.Connections == nil {
		deps.Logger.Error("remote queries tunnel execute handler not registered: the check collector is unavailable")
		return nil
	}
	service := &remotequeries.Service{Config: deps.Config, Checks: deps.Collector.GetChecks, Log: deps.Logger}
	tunnel, err := remotequeries.NewTunnel(service, tunnelAuthority{config: deps.Config, connection: deps.Connections.Current})
	if err != nil {
		deps.Logger.Errorf("remote queries tunnel execute handler not registered: %v", err)
		return nil
	}
	return NewRemoteQueriesExecuteHandler(tunnel, deps.Logger)
}
