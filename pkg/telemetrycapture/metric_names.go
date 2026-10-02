// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import "strings"

var metricNames = map[string]bool{
	"system.cpu.user": true, "system.cpu.system": true, "system.cpu.idle": true, "system.cpu.iowait": true, "system.cpu.num_cores": true,
	"system.cpu.interrupt": true, "system.cpu.context_switches": true, "system.cpu.stolen": true, "system.cpu.guest": true,
	"system.cpu.user.total": true, "system.cpu.nice.total": true, "system.cpu.system.total": true, "system.cpu.idle.total": true,
	"system.cpu.iowait.total": true, "system.cpu.irq.total": true, "system.cpu.softirq.total": true, "system.cpu.steal.total": true, "system.cpu.guest.total": true, "system.cpu.guestnice.total": true,
	"system.mem.total": true, "system.mem.used": true, "system.mem.free": true, "system.mem.usable": true, "system.mem.pct_usable": true,
	"system.mem.cached": true, "system.mem.committed": true, "system.mem.paged": true, "system.mem.nonpaged": true,
	"system.mem.pagefile.total": true, "system.mem.pagefile.used": true, "system.mem.pagefile.free": true, "system.mem.pagefile.pct_free": true,
	"system.paging.total": true, "system.paging.used": true, "system.paging.free": true, "system.paging.pct_free": true,
	"system.swap.total": true, "system.swap.used": true, "system.swap.free": true, "system.swap.pct_free": true, "system.swap.swap_in": true, "system.swap.swap_out": true,
	"system.disk.total": true, "system.disk.used": true, "system.disk.free": true, "system.disk.utilized": true, "system.uptime": true,
	"system.wlan.rssi": true, "system.wlan.noise": true, "system.wlan.txrate": true, "system.wlan.rxrate": true,
	"system.wlan.status": true, "system.wlan.roaming_events": true, "system.wlan.channel_swap_events": true, "system.wlan.check.errors": true,
	"system.battery.maximum_capacity_pct": true, "system.battery.current_charge_pct": true, "system.battery.cycle_count": true, "system.battery.charge_rate": true,
	"system.net.bytes_rcvd": true, "system.net.bytes_sent": true, "system.net.packets_in.count": true, "system.net.packets_out.count": true,
	"system.net.packets_in.drop": true, "system.net.packets_out.drop": true, "system.net.packets_in.error": true, "system.net.packets_out.error": true, "system.net.tcp.retrans_segs": true,
	"system.net.tcp.retrans_packs": true, "system.net.tcp.sent_packs": true, "system.net.tcp.rcv_packs": true,
}

// MetricAllowed preserves the capture metric scope shared by producers and sanitization.
func MetricAllowed(name string) bool { return metricNames[name] }

// MetricCheckFamily accepts only fixed check names; it never exposes a check
// instance ID, arbitrary integration name, or configuration in capture status.
func MetricCheckFamily(name string) string {
	switch name {
	case "cpu":
		return "cpu"
	case "memory":
		return "memory"
	case "disk":
		return "disk"
	case "uptime":
		return "uptime"
	case "wlan":
		return "wlan"
	case "battery":
		return "battery"
	case "network":
		return "network"
	default:
		return ""
	}
}

// MetricFamily identifies the native check family for an allowed metric.
func MetricFamily(name string) string {
	if !MetricAllowed(name) {
		return ""
	}
	switch {
	case strings.HasPrefix(name, "system.cpu."):
		return "cpu"
	case strings.HasPrefix(name, "system.mem."), strings.HasPrefix(name, "system.swap."), strings.HasPrefix(name, "system.paging."):
		return "memory"
	case strings.HasPrefix(name, "system.disk."):
		return "disk"
	case name == "system.uptime":
		return "uptime"
	case strings.HasPrefix(name, "system.wlan."):
		return "wlan"
	case strings.HasPrefix(name, "system.battery."):
		return "battery"
	case strings.HasPrefix(name, "system.net."):
		return "network"
	default:
		return ""
	}
}
