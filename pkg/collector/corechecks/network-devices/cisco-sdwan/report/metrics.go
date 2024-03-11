// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package report implements Cisco SD-WAN metadata and metrics reporting
package report

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"time"
)
import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"

// SDWanSender implements methods for sending Cisco SD-Wan metrics and metadata
type SDWanSender struct {
	sender       sender.Sender
	statsdClient *ddgostatsd.Client
}

// NewSDWanSender returns a new SDWanSender
func NewSDWanSender(sender sender.Sender, statsdClient *ddgostatsd.Client) *SDWanSender {
	return &SDWanSender{
		sender:       sender,
		statsdClient: statsdClient,
	}
}

// SendDeviceMetrics sends device hardware metrics
func (ms *SDWanSender) SendDeviceMetrics(deviceStats []client.DeviceStatistics) {
	for _, entry := range deviceStats {
		tags := []string{"device_name:" + entry.VmanageSystemIP, "test:thibaud"}
		ms.statsdClient.GaugeWithTimestamp("snmp.cpu.usage", float64(entry.CPUUserNew+entry.CPUSystemNew), append(tags, "cpu:0"), 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.memory.usage", float64(entry.MemUtil*100), append(tags, "mem:0"), 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("thibaud.test.disk.usage", float64((int64(entry.DiskUsed)/(int64(entry.DiskUsed)+entry.DiskAvail))*100), append(tags, "mem:0"), 1, time.UnixMilli(entry.EntryTime))
	}
}

// SendInterfaceMetrics sends interface metrics
func (ms *SDWanSender) SendInterfaceMetrics(interfaceStats []client.InterfaceStats) {
	for _, entry := range interfaceStats {
		tags := []string{"device_name:" + entry.VmanageSystemIP, "interface:" + entry.Interface, "test:thibaud"}
		ms.statsdClient.GaugeWithTimestamp("snmp.ifHCOutOctets", float64(entry.TxOctets), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifHCInOctets", float64(entry.RxOctets), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifHCOutOctets.rate", float64(entry.TxKbps), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifHCInOctets.rate", float64(entry.RxKbps), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifBandwidthInUsage.rate", entry.DownCapacityPercentage, tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifBandwidthOutUsage.rate", entry.UpCapacityPercentage, tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifInErrors", float64(entry.RxErrors), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifOutErrors", float64(entry.TxErrors), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifInDiscards", float64(entry.RxDrops), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("snmp.ifOutDiscards", float64(entry.TxDrops), tags, 1, time.UnixMilli(entry.EntryTime))
	}
}

// SendUptimeMetrics sends device uptime metrics
func (ms *SDWanSender) SendUptimeMetrics(uptimes map[string]float64) {
	for device, uptime := range uptimes {
		tags := []string{"device_name:" + device, "test:thibaud"}
		ms.sender.Gauge("snmp.sysUpTimeInstance", uptime, "", tags)
	}
}

// SendAppRouteMetrics send application aware routing metrics
func (ms *SDWanSender) SendAppRouteMetrics(appRouteStats []client.AppRouteStatistics) {
	for _, entry := range appRouteStats {
		tags := []string{"device_name:" + entry.VmanageSystemIP, "local_ip:" + entry.LocalSystemIP, "remote_ip:" + entry.RemoteSystemIP, "local_color:" + entry.LocalColor, "remote_color:" + entry.RemoteColor, "state:" + entry.State, "test:thibaud"}
		ms.statsdClient.GaugeWithTimestamp("thibaud.test.tunnel.latency", float64(entry.Latency), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("thibaud.test.tunnel.jitter", float64(entry.Jitter), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("thibaud.test.tunnel.loss", float64(entry.LossPercentage), tags, 1, time.UnixMilli(entry.EntryTime))
		ms.statsdClient.GaugeWithTimestamp("thibaud.test.tunnel.qoe", float64(entry.VqoeScore), tags, 1, time.UnixMilli(entry.EntryTime))
	}
}

// SendControlConnectionMetrics sends control connection state metrics
func (ms *SDWanSender) SendControlConnectionMetrics(controlConnectionsState []client.ControlConnections) {
	for _, entry := range controlConnectionsState {
		tags := []string{"device_name:" + entry.VmanageSystemIP, "control_system_ip:" + entry.SystemIP, "private_ip:" + entry.PrivateIP, "local_color:" + entry.LocalColor, "remote_color:" + entry.RemoteColor, "peer_type:" + entry.PeerType, "state:" + entry.State, "test:thibaud"}
		status := 0
		if entry.State == "up" {
			status = 1
		}
		ms.sender.Gauge("thibaud.test.control_connection.state", float64(status), "", tags)
	}
}

// SendOMPPeerMetrics send OMP Peer state metrics
func (ms *SDWanSender) SendOMPPeerMetrics(ompPeers []client.OMPPeer) {
	for _, entry := range ompPeers {
		tags := []string{"device_name:" + entry.VmanageSystemIP, "peer:" + entry.Peer, "legit:" + entry.Legit, "refresh:" + entry.Refresh, "type:" + entry.Type, "state:" + entry.State, "test:thibaud"}
		status := 0
		if entry.State == "up" {
			status = 1
		}
		ms.sender.Gauge("thibaud.test.omp.peer.state", float64(status), "", tags)
	}
}

// SendBFDSessionMetrics sends BFD session state metrics
func (ms *SDWanSender) SendBFDSessionMetrics(bfdSessionsState []client.BFDSession) {
	for _, entry := range bfdSessionsState {
		tags := []string{"device_name:" + entry.VmanageSystemIP, "local_color:" + entry.LocalColor, "remote_color:" + entry.Color, "remote_ip:" + entry.SystemIP, "proto:" + entry.Proto, "state:" + entry.State, "test:thibaud"}
		status := 0
		if entry.State == "up" {
			status = 1
		}
		ms.sender.Gauge("thibaud.test.bfd.session.state", float64(status), "", tags)
	}
}

// SendDeviceCountersMetrics sends device counters metrics
func (ms *SDWanSender) SendDeviceCountersMetrics(deviceCounters []client.DeviceCounters) {
	for _, entry := range deviceCounters {
		tags := []string{"device_name:" + entry.SystemIP}
		ms.sender.Gauge("thibaud.test.control_connection.up", float64(entry.NumberVsmartControlConnections), "", tags)
		ms.sender.Gauge("thibaud.test.control_connection.expected", float64(entry.ExpectedControlConnections), "", tags)
		ms.sender.Gauge("thibaud.test.omp.peer.up", float64(entry.OmpPeersUp), "", tags)
		ms.sender.Gauge("thibaud.test.omp.peer.down", float64(entry.OmpPeersDown), "", tags)
		ms.sender.Gauge("thibaud.test.bfd.session.up", float64(entry.BfdSessionsUp), "", tags)
		ms.sender.Gauge("thibaud.test.bfd.session.down", float64(entry.BfdSessionsDown), "", tags)
		ms.sender.Gauge("thibaud.test.crash", float64(entry.CrashCount), "", tags)
		ms.sender.Gauge("thibaud.test.reboot", float64(entry.RebootCount), "", tags)
	}
}

// Commit commits to the sender
func (ms *SDWanSender) Commit() {
	ms.sender.Commit()
}
