// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// ProcessManagerInstallerServiceRegistryKey is the "Datadog Installer" SCM service's registry
// key, whose Environment value carries the last-chosen DD_PROCESS_MANAGER_ENABLED (see
// packages.SetProcessManagerEnabled / persistProcessManagerEnabledWindows, which write to it).
// Declared here, in the lowest-level package, so that ProcessManagerEnabledFromEnv can fall back
// to the last persisted choice for every caller — not just the installer daemon's own process —
// without pkg/fleet/installer/env depending on the higher-level pkg/fleet/installer/packages
// package.
const ProcessManagerInstallerServiceRegistryKey = `SYSTEM\CurrentControlSet\Services\Datadog Installer`

func readPersistedProcessManagerEnabled() (bool, bool) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, ProcessManagerInstallerServiceRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		return false, false
	}
	defer key.Close()
	existing, _, err := key.GetStringsValue("Environment")
	if err != nil {
		return false, false
	}
	prefix := envProcessManagerEnabled + "="
	for _, entry := range existing {
		if v, ok := strings.CutPrefix(entry, prefix); ok {
			return strings.EqualFold(v, "true"), true
		}
	}
	return false, false
}
