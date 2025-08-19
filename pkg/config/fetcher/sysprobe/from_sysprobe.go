// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package sysprobe is a collection of high level helpers to pull the configuration from system-probe.
// It is separated from the other helpers in the parent package to avoid unnecessary imports in processes
// that have no need to directly communicate with system-probe.
package sysprobe

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

// SystemProbeConfig fetch the configuration from the system-probe process by querying its API
func SystemProbeConfig(config config.Reader, _ ipc.HTTPClient) (string, error) {
	hc := client.Get(config.GetString("system_probe_config.sysprobe_socket"))

	c := settingshttp.NewHTTPClient(hc, "http://localhost/config", "system-probe", settingshttp.NewHTTPClientOptions(settingshttp.CloseConnection))
	return c.FullConfig()
}

// SystemProbeConfigBySource fetch the all configuration layers from the system-probe process by querying its API
func SystemProbeConfigBySource(config config.Reader, _ ipc.HTTPClient) (string, error) {
	hc := client.Get(config.GetString("system_probe_config.sysprobe_socket"))

	c := settingshttp.NewHTTPClient(hc, "http://localhost/config", "system-probe", settingshttp.NewHTTPClientOptions(settingshttp.CloseConnection))
	return c.FullConfigBySource()
}
