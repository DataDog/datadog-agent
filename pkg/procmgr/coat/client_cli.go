// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type cliClient struct {
	cli string
}

func newCLIClient(installRoot string) Client {
	return &cliClient{cli: procmgrCLIPath(installRoot)}
}

func (c *cliClient) Connect(_ context.Context) (ProcmgrSession, error) {
	if _, err := os.Stat(c.cli); err != nil {
		return nil, err
	}
	return &cliSession{cli: c.cli}, nil
}

type cliSession struct {
	cli string
}

func (s *cliSession) Status(ctx context.Context) (DaemonSnapshot, error) {
	out, err := runProcmgrCLI(ctx, s.cli, "status", "--json")
	if err != nil {
		return DaemonSnapshot{}, err
	}
	var resp struct {
		Ready            bool   `json:"ready"`
		RunningProcesses uint32 `json:"running_processes"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return DaemonSnapshot{}, fmt.Errorf("parse dd-procmgr status output: %w", err)
	}
	return DaemonSnapshot{Reachable: true, Ready: resp.Ready, RunningProcesses: resp.RunningProcesses}, nil
}

func (s *cliSession) List(ctx context.Context) (map[string]ProcessSnapshot, error) {
	out, err := runProcmgrCLI(ctx, s.cli, "list", "--json")
	if err != nil {
		return nil, err
	}
	var items []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parse dd-procmgr list output: %w", err)
	}
	processes := make(map[string]ProcessSnapshot, len(items))
	for _, item := range items {
		processes[item.Name] = ProcessSnapshot{Name: item.Name, State: parseProcmgrState(item.State)}
	}
	return processes, nil
}

func (s *cliSession) Disconnect() error {
	return nil
}

func runProcmgrCLI(ctx context.Context, cli string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, cli, args...)
	if err := runAsDDAgent(cmd); err != nil {
		return nil, fmt.Errorf("dd-procmgr %s: %w", strings.Join(args, " "), err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "failed to connect to") {
			return nil, fmt.Errorf("dd-procmgr %s: %w: %s", strings.Join(args, " "), os.ErrNotExist, msg)
		}
		return nil, fmt.Errorf("dd-procmgr %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return stdout.Bytes(), nil
}
