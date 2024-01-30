// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fetcher is a collection of high level helpers to pull the configuration from other agent processes
package fetcher

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
)

// FetchSecurityAgentConfig fetch the configuration from the security-agent process by querying its HTTPS API
func FetchSecurityAgentConfig(config config.Reader) (string, error) {
	err := util.SetAuthToken()
	if err != nil {
		return "", err
	}

	c := util.GetClient(false)

	apiConfigURL := fmt.Sprintf("https://localhost:%v/agent/config", config.GetInt("security_agent.cmd_port"))
	client := settingshttp.NewClient(c, apiConfigURL, "security-agent")
	return client.FullConfig()
}
