// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package networkdiscovery

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/netflow/config"
)

var serverInstance *Server

// Server manages netflow listeners.
type Server struct {
	Addr   string
	config *config.NetflowConfig
}

// NewNetworkDiscoveryServer configures and returns a running SNMP traps server.
func NewNetworkDiscoveryServer(sender aggregator.Sender) (*Server, error) {
	mainConfig, err := config.ReadConfig()
	if err != nil {
		return nil, err
	}

	return &Server{
		config: mainConfig,
	}, nil
}

// Stop stops the Server.
func (s *Server) stop() {
	log.Infof("Stop NetworkDiscovery Server")
}

// StartServer starts the global NetworkDiscovery collector.
func StartServer(sender aggregator.Sender) error {
	log.Infof("Start NetworkDiscovery Server")
	server, err := NewNetworkDiscoveryServer(sender)
	serverInstance = server
	return err
}

// StopServer stops the netflow server, if it is running.
func StopServer() {
	if serverInstance != nil {
		serverInstance.stop()
		serverInstance = nil
	}
}

// IsEnabled returns whether NetworkDiscovery collection is enabled in the Agent configuration.
func IsEnabled() bool {
	return coreconfig.Datadog.GetBool("network_devices.discovery.enabled")
}
