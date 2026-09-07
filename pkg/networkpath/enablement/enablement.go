// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package enablement reports whether connection-based Network Path Dynamic Tests
// should run. It lives outside pkg/config/setup so comp/ can call it without
// violating the pkgconfigusage depguard.
package enablement

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// ConnectionDynamicTestsEnabled reports whether connection-based Network Path
// Dynamic Tests should run. Cloud Network Monitoring must be on, and either
// standard or basic tests must be enabled.
func ConnectionDynamicTestsEnabled(coreCfg, sysprobeCfg pkgconfigmodel.Reader) bool {
	return sysprobeCfg.GetBool("network_config.enabled") &&
		(coreCfg.GetBool("network_path.connections_monitoring.enabled") ||
			coreCfg.GetBool("network_path.connections_monitoring.basic_tests_enabled"))
}
