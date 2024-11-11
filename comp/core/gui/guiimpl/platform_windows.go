// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

const docURL template.URL = template.URL("https://docs.datadoghq.com/agent/basic_agent_usage/windows")
const instructionTemplate = `{{define "loginInstruction" }}
<h3>Refreshing the Session</h3>
<p>Please ensure you access the Datadog Agent Manager with one of the following:</p>
<ul>
    <li>- Right click on the Datadog Agent system tray icon -&gt; Configure, or</li>
    <li>- Run <code>launch-gui</code> command from an <strong>elevated (run as Admin)</strong> command line
		<ul>
            <li>- PowerShell: <code>&amp; "&lt;PATH_TO_AGENT.EXE&gt;" launch-gui</code></li>
            <li>- cmd: <code>"&lt;PATH_TO_AGENT.EXE&gt;" launch-gui</code></li>
        </ul>
    </li>
</ul>
<p>For more information, please visit: <u><a href="{{ .DocURL }}">{{ .DocURL }}</a></u></p>

<p>Note: If you would like to adjust the GUI session timeout, you can modify the <code>GUI_session_expiration</code> parameter in <code>datadog.yaml</code>
{{end}}`

func restartEnabled() bool {
	return true
}

// restarts the agent using the windows service manager
func restart() error {
	here, _ := executable.Folder()
	cmd := exec.Command(filepath.Join(here, "agent"), "restart-service")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("Failed to restart the agent. Error: %v", err)
	}

	return nil
}
