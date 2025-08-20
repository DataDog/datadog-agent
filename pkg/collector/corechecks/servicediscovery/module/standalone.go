// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !linux_bpf

package module

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
)

// StandaloneDiscoveryModule implements service discovery without system-probe dependencies
type StandaloneDiscoveryModule struct {
	*discovery
}

// Router interface that matches system-probe module.Router
type Router interface {
	HandleFunc(pattern string, handler http.HandlerFunc)
}

// NewStandaloneDiscoveryModule creates a new standalone discovery module
func NewStandaloneDiscoveryModule() (*StandaloneDiscoveryModule, error) {
	// Use the no-eBPF network collector factory
	networkCollectorFactory := func(_ *core.DiscoveryConfig) (core.NetworkCollector, error) {
		return &nopNetworkCollector{}, nil
	}

	// Create minimal/nil implementations for components
	discovery := newDiscoveryWithNetwork(nil, nil, core.RealTime{}, networkCollectorFactory)

	return &StandaloneDiscoveryModule{
		discovery: discovery,
	}, nil
}

// Register registers HTTP endpoints compatible with system-probe interface
func (s *StandaloneDiscoveryModule) Register(router Router) error {
	// Register the same endpoints as the system-probe module
	router.HandleFunc("/status", s.handleStatusEndpoint)
	router.HandleFunc("/state", s.handleStateEndpoint)
	router.HandleFunc("/debug", s.handleDebugEndpoint)
	router.HandleFunc("/services", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, s.handleServices))

	return nil
}

// GetStats returns module stats (compatible with module.Module interface)
func (s *StandaloneDiscoveryModule) GetStats() map[string]interface{} {
	return s.discovery.GetStats()
}

// Close cleans up resources
func (s *StandaloneDiscoveryModule) Close() {
	s.discovery.Close()
}
