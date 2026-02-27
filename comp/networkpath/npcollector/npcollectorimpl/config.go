// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/connfilter"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/pathteststore"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

type collectorConfigs struct {
	connectionsMonitoringEnabled bool
	workers                      int
	timeout                      time.Duration
	maxTTL                       int
	pathtestInputChanSize        int
	pathtestProcessingChanSize   int
	storeConfig                  pathteststore.Config
	flushInterval                time.Duration
	reverseDNSEnabled            bool
	reverseDNSTimeout            time.Duration
	disableIntraVPCCollection    bool
	networkDevicesNamespace      string
	sourceExcludedConns          map[string][]string
	destExcludedConns            map[string][]string
	tcpMethod                    payload.TCPMethod
	icmpMode                     payload.ICMPMode
	tcpSynParisTracerouteMode    bool
	tracerouteQueries            int
	e2eQueries                   int
	disableWindowsDriver         bool
	filterConfig                 []connfilter.Config
	monitorIPWithoutDomain       bool
	ddSite                       string
	sourceProduct                payload.SourceProduct
}

func newConfig(agentConfig config.Component, logger log.Component) *collectorConfigs {
	var filterConfigs []connfilter.Config
	err := structure.UnmarshalKey(agentConfig, "network_path.collector.filters", &filterConfigs)
	if err != nil {
		logger.Errorf("Error unmarshalling network_path.collector.filters: %v", err)
		filterConfigs = nil
	}
	return &collectorConfigs{
		connectionsMonitoringEnabled: agentConfig.GetBool("network_path.connections_monitoring.enabled"),
		workers:                      agentConfig.GetInt("network_path.collector.workers"),
		timeout:                      agentConfig.GetDuration("network_path.collector.timeout") * time.Millisecond,
		maxTTL:                       agentConfig.GetInt("network_path.collector.max_ttl"),
		pathtestInputChanSize:        agentConfig.GetInt("network_path.collector.input_chan_size"),
		pathtestProcessingChanSize:   agentConfig.GetInt("network_path.collector.processing_chan_size"),
		storeConfig: pathteststore.Config{
			ContextsLimit:    agentConfig.GetInt("network_path.collector.pathtest_contexts_limit"),
			TTL:              agentConfig.GetDuration("network_path.collector.pathtest_ttl"),
			Interval:         agentConfig.GetDuration("network_path.collector.pathtest_interval"),
			MaxPerMinute:     agentConfig.GetInt("network_path.collector.pathtest_max_per_minute"),
			MaxBurstDuration: agentConfig.GetDuration("network_path.collector.pathtest_max_burst_duration"),
		},
		flushInterval:             agentConfig.GetDuration("network_path.collector.flush_interval"),
		reverseDNSEnabled:         agentConfig.GetBool("network_path.collector.reverse_dns_enrichment.enabled"),
		reverseDNSTimeout:         agentConfig.GetDuration("network_path.collector.reverse_dns_enrichment.timeout") * time.Millisecond,
		disableIntraVPCCollection: agentConfig.GetBool("network_path.collector.disable_intra_vpc_collection"),
		sourceExcludedConns:       agentConfig.GetStringMapStringSlice("network_path.collector.source_excludes"),
		destExcludedConns:         agentConfig.GetStringMapStringSlice("network_path.collector.dest_excludes"),
		tcpMethod:                 payload.MakeTCPMethod(agentConfig.GetString("network_path.collector.tcp_method")),
		icmpMode:                  payload.MakeICMPMode(agentConfig.GetString("network_path.collector.icmp_mode")),
		tcpSynParisTracerouteMode: agentConfig.GetBool("network_path.collector.tcp_syn_paris_traceroute_mode"),
		tracerouteQueries:         agentConfig.GetInt("network_path.collector.traceroute_queries"),
		e2eQueries:                agentConfig.GetInt("network_path.collector.e2e_queries"),
		disableWindowsDriver:      agentConfig.GetBool("network_path.collector.disable_windows_driver"),
		networkDevicesNamespace:   agentConfig.GetString("network_devices.namespace"),
		filterConfig:              filterConfigs,
		monitorIPWithoutDomain:    agentConfig.GetBool("network_path.collector.monitor_ip_without_domain"),
		ddSite:                    agentConfig.GetString("site"),
		sourceProduct:             payload.GetSourceProduct(agentConfig.GetString("infrastructure_mode")),
	}
}

// networkPathCollectorEnabled checks if Network Path Collector should be enabled
// Network Path Collector is expected to be enabled if a feature depend on it.
func (c *collectorConfigs) networkPathCollectorEnabled() bool {
	return c.connectionsMonitoringEnabled
}
