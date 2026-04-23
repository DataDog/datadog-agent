// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package processagentunreachable

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

const (
	defaultCmdPort = 6162
	dialTimeout    = 2 * time.Second
)

// Check detects whether process collection is enabled but the process-agent is unreachable.
// Returns nil if process collection is disabled or if the process-agent is reachable.
func Check(cfg config.Component) (*healthplatform.IssueReport, error) {
	// If process collection is not enabled, there is nothing to check.
	if !cfg.GetBool("process_config.process_collection.enabled") {
		return nil, nil
	}

	port := cfg.GetInt("process_config.cmd_port")
	if port <= 0 {
		port = defaultCmdPort
	}

	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err == nil {
		// Connection succeeded — process-agent is reachable.
		conn.Close()
		return nil, nil
	}

	// Connection failed — process-agent is not running or not reachable.
	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"port":    fmt.Sprintf("%d", port),
			"enabled": "true",
		},
	}, nil
}
