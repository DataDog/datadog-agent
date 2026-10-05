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
		Version          string `json:"version"`
		UptimeSeconds    uint64 `json:"uptime_seconds"`
		TotalProcesses   uint32 `json:"total_processes"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return DaemonSnapshot{}, fmt.Errorf("parse dd-procmgr status output: %w", err)
	}
	return DaemonSnapshot{
		Reachable:        true,
		Ready:            resp.Ready,
		RunningProcesses: resp.RunningProcesses,
		Version:          resp.Version,
		UptimeSeconds:    resp.UptimeSeconds,
		TotalProcesses:   resp.TotalProcesses,
	}, nil
}

// cliProcess mirrors the JSON `dd-procmgr list --json` and `dd-procmgr describe --json` emit.
// describe fills every field; list leaves the trailing ones empty. The env map describe also
// emits is deliberately not decoded so it cannot reach a caller.
type cliProcess struct {
	Name         string   `json:"name"`
	State        string   `json:"state"`
	UUID         string   `json:"uuid"`
	PID          uint32   `json:"pid"`
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	RestartCount uint32   `json:"restart_count"`
	LastExitCode *int32   `json:"last_exit_code"`
	LastSignal   *int32   `json:"last_signal"`
	Profile      string   `json:"profile"`
	User         string   `json:"user"`

	Description         string   `json:"description"`
	WorkingDir          string   `json:"working_dir"`
	RuntimeUser         string   `json:"runtime_user"`
	RestartPolicy       string   `json:"restart_policy"`
	AutoStart           bool     `json:"auto_start"`
	ConditionPathExists string   `json:"condition_path_exists"`
	After               []string `json:"after"`
	Before              []string `json:"before"`
	Stdout              string   `json:"stdout"`
	Stderr              string   `json:"stderr"`
}

func (p cliProcess) snapshot() ProcessSnapshot {
	return ProcessSnapshot{
		Name:                p.Name,
		State:               parseProcmgrState(p.State),
		UUID:                p.UUID,
		PID:                 p.PID,
		Command:             p.Command,
		Args:                p.Args,
		RestartCount:        p.RestartCount,
		LastExitCode:        p.LastExitCode,
		LastSignal:          p.LastSignal,
		Profile:             p.Profile,
		User:                p.User,
		Description:         p.Description,
		WorkingDir:          p.WorkingDir,
		RuntimeUser:         p.RuntimeUser,
		RestartPolicy:       p.RestartPolicy,
		AutoStart:           p.AutoStart,
		ConditionPathExists: p.ConditionPathExists,
		After:               p.After,
		Before:              p.Before,
		Stdout:              p.Stdout,
		Stderr:              p.Stderr,
	}
}

func (s *cliSession) List(ctx context.Context) (map[string]ProcessSnapshot, error) {
	out, err := runProcmgrCLI(ctx, s.cli, "list", "--json")
	if err != nil {
		return nil, err
	}
	var items []cliProcess
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parse dd-procmgr list output: %w", err)
	}
	processes := make(map[string]ProcessSnapshot, len(items))
	for _, item := range items {
		processes[item.Name] = item.snapshot()
	}
	return processes, nil
}

func (s *cliSession) Describe(ctx context.Context, nameOrUUID string) (ProcessSnapshot, error) {
	out, err := runProcmgrCLI(ctx, s.cli, "describe", nameOrUUID, "--json")
	if err != nil {
		return ProcessSnapshot{}, err
	}
	var detail cliProcess
	if err := json.Unmarshal(out, &detail); err != nil {
		return ProcessSnapshot{}, fmt.Errorf("parse dd-procmgr describe output: %w", err)
	}
	return detail.snapshot(), nil
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
