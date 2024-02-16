// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fetcher is a collection of high level helpers to pull the configuration from other agent processes
package fetcher

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// SecurityAgentConfig fetch the configuration from the security-agent process by querying its HTTPS API
func SecurityAgentConfig(config config.Reader) (string, error) {
	err := util.SetAuthToken(config)
	if err != nil {
		return "", err
	}

	c := util.GetClient(false)

	apiConfigURL := fmt.Sprintf("https://localhost:%v/agent/config", config.GetInt("security_agent.cmd_port"))
	client := settingshttp.NewClient(c, apiConfigURL, "security-agent")
	return client.FullConfig()
}

// TraceAgentConfig fetch the configuration from the trace-agent process by querying its HTTPS API
func TraceAgentConfig(config config.Reader) (string, error) {
	err := util.SetAuthToken(config)
	if err != nil {
		return "", err
	}

	c := util.GetClient(false)

	port := config.GetInt("apm_config.debug.port")
	if port <= 0 {
		return "", fmt.Errorf("invalid apm_config.debug.port -- %d", port)
	}

	ipcAddressWithPort := fmt.Sprintf("http://127.0.0.1:%d/config", port)

	client := settingshttp.NewClient(c, ipcAddressWithPort, "trace-agent")
	return client.FullConfig()
}

// ProcessAgentConfig fetch the configuration from the process-agent process by querying its HTTPS API
func ProcessAgentConfig(config config.Reader, getEntireConfig bool) (string, error) {
	err := util.SetAuthToken(config)
	if err != nil {
		return "", err
	}

	c := util.GetClient(false)

	ipcAddress, err := setup.GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	port := config.GetInt("process_config.cmd_port")
	if port <= 0 {
		return "", fmt.Errorf("invalid process_config.cmd_port -- %d", port)
	}

	ipcAddressWithPort := fmt.Sprintf("http://%s:%d/config", ipcAddress, port)
	if getEntireConfig {
		ipcAddressWithPort += "/all"
	}

	client := settingshttp.NewClient(c, ipcAddressWithPort, "process-agent")

	return client.FullConfig()
}

// SystemProbeConfig fetch the configuration from the system-probe process by querying its API
func SystemProbeConfig(config config.Reader) (string, error) {
	hc := client.Get(config.GetString("system_probe_config.sysprobe_socket"))

	c := settingshttp.NewClient(hc, "http://localhost/config", "system-probe")
	return c.FullConfig()
}
