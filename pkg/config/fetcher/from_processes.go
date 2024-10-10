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
	"github.com/DataDog/datadog-agent/pkg/api/util"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
)

// SecurityAgentConfig fetch the configuration from the security-agent process by querying its HTTPS API
func SecurityAgentConfig(config config.Reader) (string, error) {
	err := util.SetAuthToken(config)
	if err != nil {
		return "", err
	}

	c := util.GetClient().WithNoVerify().WithTimeout(0).WithResolver().Build()
	c.Timeout = config.GetDuration("server_timeout") * time.Second

	apiConfigURL := fmt.Sprintf("https://%v/agent/config", util.SecurityCmd)
	client := settingshttp.NewClient(c, apiConfigURL, "security-agent", settingshttp.NewHTTPClientOptions(util.CloseConnection))
	return client.FullConfig()
}

// SecurityAgentConfigBySource fetch all configuration layers from the security-agent process by querying its HTTPS API
func SecurityAgentConfigBySource(config config.Reader) (string, error) {
	err := util.SetAuthToken(config)
	if err != nil {
		return "", err
	}

	c := util.GetClient().WithNoVerify().WithTimeout(0).WithResolver().Build()
	c.Timeout = config.GetDuration("server_timeout") * time.Second

	apiConfigURL := fmt.Sprintf("https://%v/agent/config", util.SecurityCmd)
	client := settingshttp.NewClient(c, apiConfigURL, "security-agent", settingshttp.NewHTTPClientOptions(util.CloseConnection))
	return client.FullConfigBySource()
}

// TraceAgentConfig fetch the configuration from the trace-agent process by querying its HTTPS API
func TraceAgentConfig(config config.Reader) (string, error) {
	err := util.SetAuthToken(config)
	if err != nil {
		return "", err
	}

	c := util.GetClient().WithNoVerify().WithTimeout(0).WithResolver().Build()
	c.Timeout = config.GetDuration("server_timeout") * time.Second

	ipcAddressWithPort := fmt.Sprintf("http://%v/config", util.TraceCmd)

	client := settingshttp.NewClient(c, ipcAddressWithPort, "trace-agent", settingshttp.NewHTTPClientOptions(util.CloseConnection))
	return client.FullConfig()
}

// ProcessAgentConfig fetch the configuration from the process-agent process by querying its HTTPS API
func ProcessAgentConfig(config config.Reader, getEntireConfig bool) (string, error) {
	err := util.SetAuthToken(config)
	if err != nil {
		return "", err
	}

	// ipcAddress, err := setup.GetIPCAddress(config)
	// if err != nil {
	// 	return "", err
	// }

	// port := config.GetInt("process_config.cmd_port")
	// if port <= 0 {
	// 	return "", fmt.Errorf("invalid process_config.cmd_port -- %d", port)
	// }

	ipcAddressWithPort := fmt.Sprintf("http://%v/config", util.ProcessCmd)
	if getEntireConfig {
		ipcAddressWithPort += "/all"
	}

	c := util.GetClient().WithNoVerify().WithTimeout(0).WithResolver().Build()
	c.Timeout = config.GetDuration("server_timeout") * time.Second

	client := settingshttp.NewClient(c, ipcAddressWithPort, "process-agent", settingshttp.NewHTTPClientOptions(util.CloseConnection))

	return client.FullConfig()
}
