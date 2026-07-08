// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"fmt"
	"net/http"

	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	template "github.com/DataDog/datadog-agent/pkg/template/html"
)

const docURL template.URL = template.URL("https://docs.datadoghq.com/agent/basic_agent_usage/osx")
const instructionTemplate = `{{define "loginInstruction" }}
<h3>Refreshing the Session</h3>
<p>Please ensure you access the Datadog Agent Manager with one of the following:</p>
<ul>
	<li>- through the Agent's menu bar extras icon by selecting "Open Web UI"</li>
	<li>- by running the following terminal command: "<code>datadog-agent launch-gui</code>"</li>
</ul>

<p>For more information, please visit: <u><a href="{{ .DocURL }}">{{ .DocURL }}</a></u></p>

<p>Note: If you would like to adjust the GUI session timeout, you can modify the <code>GUI_session_expiration</code> parameter in <code>datadog.yaml</code>
{{end}}`

func restartEnabled(sysprobeConfig sysprobeconfig.Component) bool {
	return sysprobeConfig.GetBool("system_probe_config.enabled")
}

func restart(getToken func() string, sysprobeSocketPath string) error {
	client := sysprobeclient.Get(sysprobeSocketPath)

	url := sysprobeclient.URL("/agent-restart")
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("could not build restart request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+getToken())

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach system-probe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := sysprobeclient.ReadAllResponseBody(resp)
		if err != nil {
			return fmt.Errorf("system-probe agent restart failed with status %d; could not read response body: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("system-probe agent restart failed with status %d: %s", resp.StatusCode, body)
	}
	return nil
}
