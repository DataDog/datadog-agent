// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

// IsEnabled reports whether the gNMI core check is enabled in agent configuration.
func IsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("network_devices.gnmi.enabled")
}
