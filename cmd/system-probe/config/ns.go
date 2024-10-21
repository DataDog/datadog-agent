// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "strings"

// spNS adds `system_probe_config` namespace to configuration key
func spNS(k ...string) string {
	return NSkey("system_probe_config", k...)
}

// netNS adds `network_config` namespace to configuration key
func netNS(k ...string) string {
	return NSkey("network_config", k...)
}

// smNS adds `service_monitoring_config` namespace to configuration key
func smNS(k ...string) string {
	return NSkey("service_monitoring_config", k...)
}

// ccmNS adds `ccm_network_config` namespace to a configuration key
func ccmNS(k ...string) string {
	return NSkey("ccm_network_config", k...)
}

// diNS adds `dynamic_instrumentation` namespace to configuration key
func diNS(k ...string) string {
	return NSkey("dynamic_instrumentation", k...)
}

// secNS adds `runtime_security_config` namespace to configuration key
func secNS(k ...string) string {
	return NSkey("runtime_security_config", k...)
}

// evNS adds `event_monitoring_config` namespace to configuration key
func evNS(k ...string) string {
	return NSkey("event_monitoring_config", k...)
}

// NSkey returns a full key path in the config file by joining the given namespace and the rest of the path fragments
func NSkey(ns string, pieces ...string) string {
	return strings.Join(append([]string{ns}, pieces...), ".")
}

// FullKeyPath returns a full key path in the config file by joining multiple path fragments
func FullKeyPath(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// wcdNS addes 'windows_crash_detection' namespace to config key
func wcdNS(k ...string) string {
	return NSkey("windows_crash_detection", k...)
}

// pngNS adds `ping` namespace to config key
func pngNS(k ...string) string {
	return NSkey("ping", k...)
}

// tracerouteNS adds `traceroute` namespace to config key
func tracerouteNS(k ...string) string {
	return NSkey("traceroute", k...)
}

// discoveryNS adds `discovery` namespace to config key
func discoveryNS(k ...string) string {
	return NSkey("discovery", k...)
}

// gpuNS adds `gpu_monitoring` namespace to config key
func gpuNS(k ...string) string {
	return NSkey("gpu_monitoring", k...)
}
