// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || netbsd || openbsd || solaris || dragonfly || linux

package guiimpl

import (
	"fmt"
)

const DocURL = "https://docs.datadoghq.com/agent/basic_agent_usage/?tab=Linux"

func restartEnabled() bool {
	return false
}

func restart() error {
	return fmt.Errorf("restarting the agent is not implemented on non-windows platforms")
}

func logginInstructions() string {
	return fmt.Sprintf(`<h3>Instructions</h3>
<p>Please ensure you access the Datadog Agent Manager by running the following bash command: "<code>datadog-agent launch-gui</code>"</p>
<p>For more information, please visit: <u><a href="%s">%s</a></u></p>

<h4>Be Aware of Token Expiration</h4>
The Datadog Agent parameter <code>GUI_session_expiration</code> (set in <code>datadog.yaml</code>) allows you to define a time expiration for the Datadog Agent Manager sessions.`,
		DocURL, DocURL)
}
