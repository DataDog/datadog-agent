// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package report implements Cisco SD-WAN metadata and metrics reporting
package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/payload"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const ciscoSDWANMetricPrefix = "cisco_sdwan."
const timestampExpiration = 6 * time.Hour

// SDWanSender implements methods for sending Cisco SD-Wan metrics and metadata
type SDWanSender struct {
	sender       sender.Sender
	namespace    string
	lastTimeSent map[string]float64
	deviceTags   map[string][]string
}

// NewSDWanSender returns a new SDWanSender
func NewSDWanSender(sender sender.Sender, namespace string) *SDWanSender {
	return &SDWanSender{
		sender:       sender,
		namespace:    namespace,
		lastTimeSent: make(map[string]float64),
	}
}

// SendDeviceMetrics sends device hardware metrics
func (ms *SDWanSender) SendDeviceMetrics(deviceStats []client.DeviceStatistics) {
	newTimestamps := make(map[string]float64)

	for _, entry := range deviceStats {
		tags := ms.getDeviceTags(entry.SystemIP)
		key := ms.getMetricKey("device_metrics", tags)

		if !ms.shouldSendEntry(key, entry.EntryTime) {
			// If the timestamp is before the max timestamp already sent, do not re-send
			continue
		}
		setNewSentTimestamp(newTimestamps, key, entry.EntryTime)

		ts := entry.EntryTime / 1000
		cpuUser := entry.CPUUserNew
		if cpuUser == 0 {
			// fallback to cpu_user
			cpuUser = entry.CPUUser
		}
		diskUsage := entry.DiskUsed / (entry.DiskUsed + entry.DiskAvail) * 100

		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"cpu.usage", cpuUser+entry.CPUSystem, tags, ts) // Using CPUUserNew and CPUSystem (not new...) to match vManage UI
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"memory.usage", entry.MemUtil*100, tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"disk.usage", diskUsage, tags, ts)
	}

	ms.updateTimestamps(newTimestamps)
}

// SendInterfaceMetrics sends interface metrics
func (ms *SDWanSender) SendInterfaceMetrics(interfaceStats []client.InterfaceStats, interfacesMap map[string]payload.CiscoInterface) {
	newTimestamps := make(map[string]float64)

	for _, entry := range interfaceStats {
		deviceTags := ms.getDeviceTags(entry.VmanageSystemIP)
		interfaceTags := []string{
			"interface:" + entry.Interface,
			fmt.Sprintf("vpn_id:%d", int(entry.VpnID)),
		}

		itfID := fmt.Sprintf("%s:%s", entry.VmanageSystemIP, entry.Interface)
		itf, foundInterface := interfacesMap[itfID]

		tags := append(deviceTags, interfaceTags...)

		if foundInterface {
			index, err := itf.Index()
			if err == nil {
				tags = append(tags, fmt.Sprintf("interface_index:%d", index))
			}
			statusTags := append(tags, "oper_status:"+itf.OperStatus().AsString(), "admin_status:"+itf.AdminStatus().AsString())

			ms.sender.Gauge(ciscoSDWANMetricPrefix+"interface.status", 1, "", statusTags)
			ms.sender.Gauge(ciscoSDWANMetricPrefix+"interface.speed", float64(itf.GetSpeedMbps()*1000), "", tags)
		}

		key := ms.getMetricKey("interface_metrics", tags)

		if !ms.shouldSendEntry(key, entry.EntryTime) {
			// If the timestamp is before the max timestamp already sent, do not re-send
			continue
		}
		setNewSentTimestamp(newTimestamps, key, entry.EntryTime)

		ts := entry.EntryTime / 1000

		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"interface.tx_bits", entry.TxOctets*8, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"interface.rx_bits", entry.RxOctets*8, tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"interface.rx_kbps", entry.RxKbps, tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"interface.tx_kbps", entry.TxKbps, tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"interface.rx_bandwidth_usage", entry.DownCapacityPercentage, tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"interface.tx_bandwidth_usage", entry.UpCapacityPercentage, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"interface.rx_errors", entry.RxErrors, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"interface.tx_errors", entry.TxErrors, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"interface.rx_drops", entry.RxDrops, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"interface.tx_drops", entry.TxDrops, tags, ts)
	}

	ms.updateTimestamps(newTimestamps)
}

// SendUptimeMetrics sends device uptime metrics
func (ms *SDWanSender) SendUptimeMetrics(uptimes map[string]float64) {
	for device, uptime := range uptimes {
		tags := ms.getDeviceTags(device)
		ms.sender.Gauge(ciscoSDWANMetricPrefix+"device.uptime", uptime, "", tags)
	}
}

// SendAppRouteMetrics send application aware routing metrics
func (ms *SDWanSender) SendAppRouteMetrics(appRouteStats []client.AppRouteStatistics) {
	newTimestamps := make(map[string]float64)

	for _, entry := range appRouteStats {
		deviceTags := ms.getDeviceTags(entry.VmanageSystemIP)
		remoteTags := ms.getRemoteDeviceTags(entry.RemoteSystemIP)

		tags := append(deviceTags, remoteTags...)
		tags = append(tags, "local_color:"+entry.LocalColor, "remote_color:"+entry.RemoteColor, "state:"+entry.State)
		key := ms.getMetricKey("tunnel_metrics", tags)

		if !ms.shouldSendEntry(key, entry.EntryTime) {
			// If the timestamp is before the max timestamp already sent, do not re-send
			continue
		}
		setNewSentTimestamp(newTimestamps, key, entry.EntryTime)

		ts := entry.EntryTime / 1000
		status := 0
		if entry.State == "Up" {
			status = 1
		}

		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.status", float64(status), tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.latency", entry.Latency, tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.jitter", entry.Jitter, tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.loss", entry.LossPercentage, tags, ts)
		ms.gaugeWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.qoe", entry.VqoeScore, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.rx_bits", entry.RxOctets*8, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.tx_bits", entry.TxOctets*8, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.rx_packets", entry.RxPkts, tags, ts)
		ms.countWithTimestamp(ciscoSDWANMetricPrefix+"tunnel.tx_packets", entry.TxPkts, tags, ts)
	}

	ms.updateTimestamps(newTimestamps)
}

// SendControlConnectionMetrics sends control connection state metrics
func (ms *SDWanSender) SendControlConnectionMetrics(controlConnectionsState []client.ControlConnections) {
	for _, entry := range controlConnectionsState {
		deviceTags := ms.getDeviceTags(entry.VmanageSystemIP)
		remoteTags := ms.getRemoteDeviceTags(entry.SystemIP)

		tags := append(deviceTags, remoteTags...)
		tags = append(tags, "private_ip:"+entry.PrivateIP, "local_color:"+entry.LocalColor, "remote_color:"+entry.RemoteColor, "peer_type:"+entry.PeerType, "state:"+entry.State)

		status := 0
		if entry.State == "up" {
			status = 1
		}
		ms.sender.Gauge(ciscoSDWANMetricPrefix+"control_connection.status", float64(status), "", tags)
	}
}

// SendOMPPeerMetrics send OMP Peer state metrics
func (ms *SDWanSender) SendOMPPeerMetrics(ompPeers []client.OMPPeer) {
	for _, entry := range ompPeers {
		deviceTags := ms.getDeviceTags(entry.VmanageSystemIP)
		remoteTags := ms.getRemoteDeviceTags(entry.Peer)

		tags := append(deviceTags, remoteTags...)
		tags = append(tags, "legit:"+entry.Legit, "refresh:"+entry.Refresh, "type:"+entry.Type, "state:"+entry.State)

		status := 0
		if entry.State == "up" {
			status = 1
		}
		ms.sender.Gauge(ciscoSDWANMetricPrefix+"omp_peer.status", float64(status), "", tags)
	}
}

// SendBFDSessionMetrics sends BFD session state metrics
func (ms *SDWanSender) SendBFDSessionMetrics(bfdSessionsState []client.BFDSession) {
	for _, entry := range bfdSessionsState {
		deviceTags := ms.getDeviceTags(entry.VmanageSystemIP)
		remoteTags := ms.getRemoteDeviceTags(entry.SystemIP)

		tags := append(deviceTags, remoteTags...)
		tags = append(tags, "local_color:"+entry.LocalColor, "remote_color:"+entry.Color, "proto:"+entry.Proto, "state:"+entry.State)

		status := 0
		if entry.State == "up" {
			status = 1
		}
		ms.sender.Gauge(ciscoSDWANMetricPrefix+"bfd_session.status", float64(status), "", tags)
	}
}

// SendDeviceCountersMetrics sends device counters metrics
func (ms *SDWanSender) SendDeviceCountersMetrics(deviceCounters []client.DeviceCounters) {
	for _, entry := range deviceCounters {
		tags := ms.getDeviceTags(entry.SystemIP)

		ms.sender.MonotonicCount(ciscoSDWANMetricPrefix+"crash.count", float64(entry.CrashCount), "", tags)
		ms.sender.MonotonicCount(ciscoSDWANMetricPrefix+"reboot.count", float64(entry.RebootCount), "", tags)
	}
}

// Commit commits to the sender
func (ms *SDWanSender) Commit() {
	ms.sender.Commit()
	ms.expireTimeSent()
}

// gaugeWithTimestamp wraps sender GaugeWithTimestamp with error handling
func (ms *SDWanSender) gaugeWithTimestamp(name string, value float64, tags []string, ts float64) {
	err := ms.sender.GaugeWithTimestamp(name, value, "", tags, ts)
	if err != nil {
		log.Warnf("Error sending Cisco SD-WAN metric %s : %s", name, err)
	}
}

// countWithTimestamp wraps sender CountWithTimestamp with error handling
func (ms *SDWanSender) countWithTimestamp(name string, value float64, tags []string, ts float64) {
	err := ms.sender.CountWithTimestamp(name, value, "", tags, ts)
	if err != nil {
		log.Warnf("Error sending Cisco SD-WAN metric %s : %s", name, err)
	}
}

func (ms *SDWanSender) getMetricKey(metric string, tags []string) string {
	return metric + ":" + strings.Join(tags, ",")
}

func (ms *SDWanSender) shouldSendEntry(key string, ts float64) bool {
	lastTs, ok := ms.lastTimeSent[key]
	if ok && lastTs >= ts {
		return false
	}
	return true
}

// setNewSentTimestamp is a util to set new timestamps
func setNewSentTimestamp(newTimestamps map[string]float64, key string, ts float64) {
	lastTs := newTimestamps[key]
	if lastTs > ts {
		return
	}
	newTimestamps[key] = ts
}

func (ms *SDWanSender) updateTimestamps(newTimestamps map[string]float64) {
	for key, ts := range newTimestamps {
		ms.lastTimeSent[key] = ts
	}
}

func (ms *SDWanSender) expireTimeSent() {
	expireTs := TimeNow().Add(-timestampExpiration).UTC().Unix()
	for key, ts := range ms.lastTimeSent {
		if ts < float64(expireTs) {
			delete(ms.lastTimeSent, key)
		}
	}
}

// SetDeviceTags sets the device tags map
func (ms *SDWanSender) SetDeviceTags(deviceTags map[string][]string) {
	ms.deviceTags = deviceTags
}

func (ms *SDWanSender) getDeviceTags(systemIP string) []string {
	tags, ok := ms.deviceTags[systemIP]
	if !ok {
		return []string{"system_ip:" + systemIP}
	}
	return tags
}

func (ms *SDWanSender) getRemoteDeviceTags(systemIP string) []string {
	tags := ms.getDeviceTags(systemIP)

	var remoteTags []string
	for _, tag := range tags {
		if strings.HasPrefix(tag, "device_namespace") {
			// No need to tag remote devices by namespace
			continue
		}
		remoteTags = append(remoteTags, "remote_"+tag)
	}

	return remoteTags
}
