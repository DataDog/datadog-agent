// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"sync"

	model "github.com/DataDog/agent-payload/v5/process"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
)

var (
	cfgOnce  = sync.Once{}
	agentCfg *model.AgentConfiguration
)

// ConnectionsModeler contains all the necessary structs for modeling a connection.
type ConnectionsModeler struct {
	usmEncoders  []usmEncoder
	dnsFormatter *dnsFormatter
	ipc          ipCache
	routeIndex   map[network.Via]RouteIdx
	tagsSet      *network.TagsSet
}

// NewConnectionsModeler initializes the connection modeler with encoders, dns formatter for
// the existing connections. The ConnectionsModeler holds the traffic encoders grouped by USM logic.
// It also includes formatted connection telemetry related to all batches, not specific batches.
// Furthermore, it stores the current agent configuration which applies to all instances related to the entire set of connections,
// rather than just individual batches.
func NewConnectionsModeler(conns *network.Connections) *ConnectionsModeler {
	ipc := make(ipCache, len(conns.Conns)/2)
	return &ConnectionsModeler{
		usmEncoders:  initializeUSMEncoders(conns),
		ipc:          ipc,
		dnsFormatter: newDNSFormatter(conns, ipc),
		routeIndex:   make(map[network.Via]RouteIdx),
		tagsSet:      network.NewTagsSet(),
	}
}

// Close cleans all encoders resources.
func (c *ConnectionsModeler) Close() {
	for _, encoder := range c.usmEncoders {
		encoder.Close()
	}
}

func (c *ConnectionsModeler) modelConnections(builder *model.ConnectionsBuilder, conns *network.Connections) {
	cfgOnce.Do(func() {
		agentCfg = &model.AgentConfiguration{
			NpmEnabled: pkgconfigsetup.SystemProbe().GetBool("network_config.enabled"),
			UsmEnabled: pkgconfigsetup.SystemProbe().GetBool("service_monitoring_config.enabled"),
			CcmEnabled: pkgconfigsetup.SystemProbe().GetBool("ccm_network_config.enabled"),
			CsmEnabled: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.enabled"),
		}
	})

	for _, conn := range conns.Conns {
		builder.AddConns(func(builder *model.ConnectionBuilder) {
			FormatConnection(builder, conn, c.routeIndex, c.usmEncoders, c.dnsFormatter, c.ipc, c.tagsSet)
		})
	}

	routes := make([]*model.Route, len(c.routeIndex))
	for _, v := range c.routeIndex {
		routes[v.Idx] = &v.Route
	}

	builder.SetAgentConfiguration(func(w *model.AgentConfigurationBuilder) {
		w.SetDsmEnabled(agentCfg.DsmEnabled)
		w.SetNpmEnabled(agentCfg.NpmEnabled)
		w.SetUsmEnabled(agentCfg.UsmEnabled)
		w.SetCcmEnabled(agentCfg.CcmEnabled)
		w.SetCsmEnabled(agentCfg.CsmEnabled)
	})
	for _, d := range c.dnsFormatter.Domains() {
		builder.AddDomains(d)
	}

	for _, route := range routes {
		builder.AddRoutes(func(w *model.RouteBuilder) {
			if route.Subnet != nil {
				w.SetSubnet(func(w *model.SubnetBuilder) {
					w.SetAlias(route.Subnet.Alias)
				})
			}
			if route.Interface != nil {
				w.SetInterface(func(w *model.InterfaceBuilder) {
					w.SetHardwareAddr(route.Interface.HardwareAddr)
				})
			}
		})
	}

	c.dnsFormatter.FormatDNS(builder)

	for _, tag := range c.tagsSet.GetStrings() {
		builder.AddTags(tag)
	}

	FormatConnectionTelemetry(builder, conns.ConnTelemetry)
	FormatCompilationTelemetry(builder, conns.CompilationTelemetryByAsset)
	FormatCORETelemetry(builder, conns.CORETelemetryByAsset)
	builder.SetKernelHeaderFetchResult(uint64(conns.KernelHeaderFetchResult))
	for _, asset := range conns.PrebuiltAssets {
		builder.AddPrebuiltEBPFAssets(asset)
	}

}
