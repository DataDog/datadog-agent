// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// configCheckURL contains the Agent API endpoint URL exposing the loaded checks
var configCheckURL string

// GetConfigCheck dump all loaded configurations to the writer
func GetConfigCheck(w io.Writer, withDebug bool) error {
	if w != color.Output {
		color.NoColor = true
	}

	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	err := util.SetAuthToken(config.Datadog)
	if err != nil {
		return err
	}
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	if configCheckURL == "" {
		configCheckURL = fmt.Sprintf("https://%v:%v/agent/config-check", ipcAddress, config.Datadog.GetInt("cmd_port"))
	}
	r, err := util.DoGet(c, configCheckURL, util.LeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			return fmt.Errorf("the agent ran into an error while checking config: %s", string(r))
		}
		return fmt.Errorf("failed to query the agent (running?): %s", err)
	}

	return nil
}

// GetClusterAgentConfigCheck proxies GetConfigCheck overidding the URL
func GetClusterAgentConfigCheck(w io.Writer, withDebug bool) error {
	configCheckURL = fmt.Sprintf("https://localhost:%v/config-check", config.Datadog.GetInt("cluster_agent.cmd_port"))
	return GetConfigCheck(w, withDebug)
}
