// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package integrationdetection

// integrationForCheck returns the canonical Datadog integration name for a given
// Autodiscovery check name (config.Name). The check name is NOT the daemon
// binary name — e.g. "redisdb", not "redis-server". Verify actual names by
// running `agent status` with each integration enabled.
//
// Returns ("", false) when the check name is not in the allowlist.
// Adding a new integration requires only a new case here.
//
// TODO: extend this list as additional integrations are validated against real agent runs.
func integrationForCheck(checkName string) (string, bool) {
	switch checkName {
	case "redisdb":
		return "redis", true
	case "elastic":
		return "elasticsearch", true
	case "nginx":
		return "nginx", true
	case "etcd":
		return "etcd", true
	default:
		return "", false
	}
}
