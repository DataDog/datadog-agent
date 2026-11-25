// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !test

package rcx509clientimpl

import (
	"fmt"

	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/libdd-rc/ffi-hosts/go/rcx509"
)

const debugServicePingURI = "rc.x509.magic_tunnel.remote_config.v1.DebugService/Ping"

// NewClientFactory returns the production adapter for the public libdd-rc Go
// client. Keeping this boundary injectable lets the component test lifecycle
// behavior without initializing the native x509 subsystem. It must be
// registered with fxutil.ProvideComponentConstructor.
func NewClientFactory(deps ClientFactoryDependencies) ClientFactoryProvides {
	logger, connections := deps.Logger, deps.Connections
	return ClientFactoryProvides{Factory: func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
		// The sink is process-global and installed before the client exists so
		// that no handshake is missed. It is never touched unless asked for:
		// the connection ID is captured when it is logged or when the Execute
		// handler is to be registered (and published in inventory metadata).
		var reporter *connectionReporter
		if config.ReportConnectionID || config.ExecuteHandler != nil {
			reporter = newConnectionReporter(logger, rcx509.EnableLogSink)
			reporter.tracker = connections
			reporter.logConnection = config.ReportConnectionID
			if config.ExecuteHandler != nil {
				var checks func() []check.Check
				if deps.Collector != nil {
					checks = deps.Collector.GetChecks
				}
				reporter.publisher = newTunnelMetadataPublisher(deps.Inventory, checks)
				if reporter.publisher == nil {
					logger.Warn("remote queries tunnel connection ID will not be published in check inventory metadata: the inventory or the check collector is unavailable")
				}
			}
			if !reporter.install() {
				reporter = nil
			}
		}

		client, err := rcx509.NewClient(
			config.URL,
			config.AppName,
			config.Version,
			rcx509.WithAPIKey(config.APIKey),
			rcx509.WithHTTPClient(config.HTTPClient),
		)
		if err != nil {
			reporter.close()
			return nil, err
		}

		if config.DebugPingEnabled {
			if err := client.RegisterHandler(debugServicePingURI, newDebugPingHandler(logger)); err != nil {
				_ = client.Close()
				reporter.close()
				return nil, fmt.Errorf("register x509 debug ping handler: %w", err)
			}
			logger.Info("remote config x509 debug ping handler enabled for manual staging validation")
		}

		if config.ExecuteHandler != nil && reporter == nil {
			// Without the reporter the Agent cannot know its connection ID, so
			// the authority would refuse every dispatch. Register nothing.
			logger.Error("remote queries tunnel execute handler not registered: the connection ID reporter is unavailable")
		} else if config.ExecuteHandler != nil {
			if err := client.RegisterHandler(remoteQueriesExecuteURI, rcx509.HandlerFunc(config.ExecuteHandler)); err != nil {
				_ = client.Close()
				reporter.close()
				return nil, fmt.Errorf("register x509 remote queries execute handler: %w", err)
			}
			reporter.markExecuteHandlerRegistered()
			logger.Info("remote config x509 remote queries execute handler registered")
		}

		if reporter != nil {
			return &reportingClient{Client: client, reporter: reporter}, nil
		}
		return client, nil
	}}
}
