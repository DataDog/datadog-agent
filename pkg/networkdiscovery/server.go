// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package networkdiscovery

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/config"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/discoverycollector"
	coreutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var serverInstance *Server

// Server manages netflow listeners.
type Server struct {
	Addr      string
	config    *config.NetworkDiscoveryConfig
	collector *discoverycollector.DiscoveryCollector
}

// NewNetworkDiscoveryServer configures and returns a running SNMP traps server.
func NewNetworkDiscoveryServer(sender aggregator.Sender) (*Server, error) {
	mainConfig, err := config.ReadConfig()
	if err != nil {
		return nil, err
	}

	hostname, err := coreutil.GetHostname(context.TODO())
	if err != nil {
		log.Warnf("Error getting the hostname: %v", err)
		hostname = ""
	}

	collector := discoverycollector.NewDiscoveryCollector(sender, mainConfig, hostname)
	go collector.Start()

	return &Server{
		config:    mainConfig,
		collector: collector,
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
