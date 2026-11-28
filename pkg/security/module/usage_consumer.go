// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	sbomapi "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// UsageConsumer serves the core agent the runtime usage of packages that the
// SBOM resolver records. It runs whenever sbom.enrichment.usage.enabled is on,
// next to the CWS consumer where CWS runs. Alone, it brings up the security
// probe and its SBOM resolver, and leaves the rule engine and the billing
// metrics to the CWS consumer, so that CWS billing covers the hosts that run
// CWS.
type UsageConsumer struct {
	*CommandServer
	config *config.RuntimeSecurityConfig
	probe  *probe.Probe
}

// NewUsageConsumer initializes the UsageConsumer
func NewUsageConsumer(cmdServer *CommandServer, evm *eventmonitor.EventMonitor, cfg *config.RuntimeSecurityConfig, stopChan chan struct{}) (*UsageConsumer, error) {
	c := &UsageConsumer{
		CommandServer: cmdServer,
		config:        cfg,
		probe:         evm.Probe,
	}

	sbomServer := NewSBOMAPIServer(evm.Probe, stopChan)

	seclog.Debugf("Registering SBOM collector server")
	sbomapi.RegisterSBOMCollectorServer(c.grpcCmdServer.ServiceRegistrar(), sbomServer)

	return c, nil
}

// ID returns the identifier for the usage consumer
func (u *UsageConsumer) ID() string {
	return "USAGE"
}

// Start starts the usage consumer
func (u *UsageConsumer) Start() error {
	if err := u.CommandServer.Start(); err != nil {
		return err
	}
	seclog.Infof("usage consumer started")
	return nil
}

// Stop stops the usage consumer
func (u *UsageConsumer) Stop() {
	u.CommandServer.Stop()
	seclog.Infof("usage consumer stopped")
}
