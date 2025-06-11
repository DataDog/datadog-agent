// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fetcher is a collection of high level helpers to pull the configuration from other agent processes
package fetcher

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// SecurityAgentConfig fetch the configuration from the security-agent process by querying its HTTPS API
func SecurityAgentConfig(config config.Reader, client ipc.HTTPClient) (string, error) {
	port := config.GetInt("security_agent.cmd_port")
	if port <= 0 {
		return "", fmt.Errorf("invalid security_agent.cmd_port -- %d", port)
	}

	timeout := config.GetDuration("server_timeout") * time.Second

	apiConfigURL := fmt.Sprintf("https://localhost:%v/agent/config", port)

	settingsClient := settingshttp.NewHTTPSClient(client, apiConfigURL, "security-agent", ipchttp.WithTimeout(timeout))
	return settingsClient.FullConfig()
}

// SecurityAgentConfigBySource fetch all configuration layers from the security-agent process by querying its HTTPS API
func SecurityAgentConfigBySource(config config.Reader, client ipc.HTTPClient) (string, error) {
	port := config.GetInt("security_agent.cmd_port")
	if port <= 0 {
		return "", fmt.Errorf("invalid security_agent.cmd_port -- %d", port)
	}

	timeout := config.GetDuration("server_timeout") * time.Second

	apiConfigURL := fmt.Sprintf("https://localhost:%v/agent/config", port)

	settingsClient := settingshttp.NewHTTPSClient(client, apiConfigURL, "security-agent", ipchttp.WithTimeout(timeout))
	return settingsClient.FullConfigBySource()
}

// TraceAgentConfig fetch the configuration from the trace-agent process by querying its HTTPS API
func TraceAgentConfig(config config.Reader, client ipc.HTTPClient) (string, error) {
	port := config.GetInt("apm_config.debug.port")
	if port <= 0 {
		return "", fmt.Errorf("invalid apm_config.debug.port -- %d", port)
	}

	timeout := config.GetDuration("server_timeout") * time.Second

	ipcAddressWithPort := fmt.Sprintf("https://127.0.0.1:%d/config", port)

	settingsClient := settingshttp.NewHTTPSClient(client, ipcAddressWithPort, "trace-agent", ipchttp.WithTimeout(timeout))
	return settingsClient.FullConfig()
}

// ProcessAgentConfig fetch the configuration from the process-agent process by querying its HTTPS API
func ProcessAgentConfig(config config.Reader, client ipc.HTTPClient, getEntireConfig bool) (string, error) {
	ipcAddress, err := setup.GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	port := config.GetInt("process_config.cmd_port")
	if port <= 0 {
		return "", fmt.Errorf("invalid process_config.cmd_port -- %d", port)
	}

	ipcAddressWithPort := fmt.Sprintf("https://%s:%d/config", ipcAddress, port)
	if getEntireConfig {
		ipcAddressWithPort += "/all"
	}

	timeout := config.GetDuration("server_timeout") * time.Second

	settingsClient := settingshttp.NewHTTPSClient(client, ipcAddressWithPort, "process-agent", ipchttp.WithTimeout(timeout))
	return settingsClient.FullConfig()
}
