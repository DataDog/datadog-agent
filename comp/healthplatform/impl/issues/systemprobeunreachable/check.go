// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package systemprobeunreachable

import (
	"net"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
)

const (
	defaultSocketPath = "/var/run/sysprobe/sysprobe.sock"
	dialTimeout       = 2 * time.Second
)

// Check detects whether NPM/USM is enabled but system-probe is not reachable.
// Returns a non-nil IssueReport if the socket is unreachable, nil otherwise.
func Check(cfg config.Component) (*healthplatform.IssueReport, error) {
	npmEnabled := cfg.GetBool("network_config.enabled")
	usmEnabled := cfg.GetBool("service_monitoring_config.enabled")

	if !npmEnabled && !usmEnabled {
		return nil, nil
	}

	socketPath := cfg.GetString("system_probe_config.sysprobe_socket")
	if socketPath == "" {
		socketPath = defaultSocketPath
	}

	conn, err := net.DialTimeout("unix", socketPath, dialTimeout)
	if err == nil {
		conn.Close()
		return nil, nil
	}

	networkEnabled := "false"
	if npmEnabled {
		networkEnabled = "true"
	}

	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"socket":          socketPath,
			"network_enabled": networkEnabled,
		},
	}, nil
}
