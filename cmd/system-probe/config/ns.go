// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "strings"

// spNS adds `system_probe_config` namespace to configuration key
func spNS(k ...string) string {
	return nskey("system_probe_config", k...)
}

// netNS adds `network_config` namespace to configuration key
func netNS(k ...string) string {
	return nskey("network_config", k...)
}

// smNS adds `service_monitoring_config` namespace to configuration key
func smNS(k ...string) string {
	return nskey("service_monitoring_config", k...)
}

// dsmNS adds `data_streams_config` namespace to configuration key
func dsmNS(k ...string) string {
	return nskey("data_streams_config", k...)
}

// diNS adds `dynamic_instrumentation` namespace to configuration key
func diNS(k ...string) string {
	return nskey("dynamic_instrumentation", k...)
}

// secNS adds `runtime_security_config` namespace to configuration key
func secNS(k ...string) string {
	return nskey("runtime_security_config", k...)
}

// evNS adds `event_monitoring_config` namespace to configuration key
func evNS(k ...string) string {
	return nskey("event_monitoring_config", k...)
}

func nskey(ns string, pieces ...string) string {
	return strings.Join(append([]string{ns}, pieces...), ".")
}

// wcdNS addes 'windows_crash_detection' namespace to config key
func wcdNS(k ...string) string {
	return nskey("windows_crash_detection", k...)
}
