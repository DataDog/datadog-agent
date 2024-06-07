// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"fmt"
	"io"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// GetClusterAgentConfigCheck gets config check from the server
func GetClusterAgentConfigCheck(w io.Writer, noColor bool, withDebug bool) error {
	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	err := util.SetAuthToken(config.Datadog())
	if err != nil {
		return err
	}

	v := url.Values{}
	if withDebug {
		v.Set("verbose", "true")
	}

	if noColor {
		v.Set("nocolor", "true")
	} else {
		v.Set("nocolor", "false")
	}

	targetURL := url.URL{
		Scheme:   "https",
		Host:     fmt.Sprintf("localhost:%v", config.Datadog().GetInt("cluster_agent.cmd_port")),
		Path:     "config-check",
		RawQuery: v.Encode(),
	}

	r, err := util.DoGet(c, targetURL.String(), util.LeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			return fmt.Errorf("the agent ran into an error while checking config: %s", string(r))
		}
		return fmt.Errorf("failed to query the agent (running?): %s", err)
	}

	_, err = w.Write(r)

	return err
}
