// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "strings"

func spNS(k ...string) string {
	return nskey("system_probe_config", k...)
}

func netNS(k ...string) string {
	return nskey("network_config", k...)
}

func smNS(k ...string) string {
	return nskey("service_monitoring_config", k...)
}

func dsmNS(k ...string) string {
	return nskey("data_streams_config", k...)
}

func diNS(k ...string) string {
	return nskey("dynamic_instrumentation", k...)
}

func secNS(k ...string) string {
	return nskey("runtime_security_config", k...)
}

func evNS(k ...string) string {
	return nskey("event_monitoring_config", k...)
}

func nskey(ns string, pieces ...string) string {
	return strings.Join(append([]string{ns}, pieces...), ".")
}
