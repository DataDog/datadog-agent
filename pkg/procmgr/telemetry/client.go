// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Client queries dd-procmgrd state. The default implementation shells out to dd-procmgr.
type Client interface {
	DaemonStatus(ctx context.Context) (DaemonSnapshot, error)
	ListProcesses(ctx context.Context) (map[string]ProcessSnapshot, error)
}

type cliClient struct {
	cliPath    string
	socketPath string
}

func newCLIClient(installRoot string) Client {
	return &cliClient{
		cliPath:    procmgrCLIPath(installRoot),
		socketPath: defaultProcmgrSocket,
	}
}

func (c *cliClient) DaemonStatus(ctx context.Context) (DaemonSnapshot, error) {
	out, err := c.run(ctx, "status")
	if err != nil {
		return DaemonSnapshot{}, err
	}

	var payload struct {
		Ready            bool   `json:"ready"`
		RunningProcesses uint32 `json:"running_processes"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return DaemonSnapshot{}, fmt.Errorf("parse procmgr status: %w", err)
	}

	return DaemonSnapshot{
		Reachable:        true,
		Ready:            payload.Ready,
		RunningProcesses: payload.RunningProcesses,
	}, nil
}

func (c *cliClient) ListProcesses(ctx context.Context) (map[string]ProcessSnapshot, error) {
	out, err := c.run(ctx, "list")
	if err != nil {
		return nil, err
	}

	var items []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parse procmgr list: %w", err)
	}

	processes := make(map[string]ProcessSnapshot, len(items))
	for _, item := range items {
		processes[item.Name] = ProcessSnapshot{
			Name:  item.Name,
			State: item.State,
		}
	}
	return processes, nil
}

func (c *cliClient) run(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := os.Stat(c.cliPath); err != nil {
		return nil, err
	}

	cmdArgs := append([]string{"--json"}, args...)
	cmd := exec.CommandContext(ctx, c.cliPath, cmdArgs...)
	cmd.Env = append(os.Environ(), "DD_PM_SOCKET_PATH="+c.socketPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, string(out))
	}
	return out, nil
}

const clientTimeout = 5 * time.Second

func clientContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, clientTimeout)
}
