// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"fmt"
	"html/template"
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

func restartEnabled() bool {
	return false
}

func restart() error {
	return fmt.Errorf("restarting the agent is not implemented on non-windows platforms")
}
