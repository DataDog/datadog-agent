// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securitycontext

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SeccompAction is the effective action for a syscall.
type SeccompAction string

// Seccomp actions.
const (
	ActionAllow       SeccompAction = "ALLOW"
	ActionErrno       SeccompAction = "ERRNO"
	ActionKillThread  SeccompAction = "KILL_THREAD"
	ActionKillProcess SeccompAction = "KILL_PROCESS"
	ActionTrap        SeccompAction = "TRAP"
	ActionTrace       SeccompAction = "TRACE"
	ActionLog         SeccompAction = "LOG"
	ActionNotify      SeccompAction = "USER_NOTIF"
)

// ArgCondition describes one argument-level check on a syscall.
type ArgCondition struct {
	Index  int
	Op     string
	Value  uint64
	Action SeccompAction
}

// SyscallRule is the action for a syscall, optionally with argument conditions.
type SyscallRule struct {
	Action        SeccompAction
	ArgConditions []ArgCondition
}

// SeccompFilterResult holds a resolved seccomp filter.
type SeccompFilterResult struct {
	DefaultAction SeccompAction
	Syscalls      map[string]SyscallRule
}

// defaultSeccompRoot is the kubelet seccomp root that Localhost profile paths
// are resolved against.
const defaultSeccompRoot = "/var/lib/kubelet/seccomp"

// ociSeccompProfile mirrors the on-disk Kubernetes/OCI seccomp profile JSON.
type ociSeccompProfile struct {
	DefaultAction string              `json:"defaultAction"`
	Syscalls      []ociSeccompSyscall `json:"syscalls"`
}

type ociSeccompSyscall struct {
	Names  []string        `json:"names"`
	Action string          `json:"action"`
	Args   []ociSeccompArg `json:"args"`
}

type ociSeccompArg struct {
	Index uint   `json:"index"`
	Value uint64 `json:"value"`
	Op    string `json:"op"`
}

// readLocalhostSeccompProfile reads and parses the Localhost seccomp profile
// referenced by localhostPath (relative to the kubelet seccomp root).
func readLocalhostSeccompProfile(localhostPath string) (*SeccompFilterResult, error) {
	if localhostPath == "" {
		return nil, fmt.Errorf("empty localhost profile path")
	}
	path := seccompProfilePath(localhostPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seccomp profile %s: %w", path, err)
	}
	var prof ociSeccompProfile
	if err := json.Unmarshal(data, &prof); err != nil {
		return nil, fmt.Errorf("parse seccomp profile %s: %w", path, err)
	}
	return ociToFilterResult(&prof), nil
}

func seccompProfilePath(localhostPath string) string {
	root := defaultSeccompRoot
	if hostRoot := os.Getenv("HOST_ROOT"); hostRoot != "" {
		root = filepath.Join(hostRoot, root)
	}
	return filepath.Join(root, localhostPath)
}

func ociToFilterResult(prof *ociSeccompProfile) *SeccompFilterResult {
	res := &SeccompFilterResult{
		DefaultAction: scmpActionToSeccompAction(prof.DefaultAction),
		Syscalls:      make(map[string]SyscallRule, len(prof.Syscalls)),
	}
	for _, sc := range prof.Syscalls {
		action := scmpActionToSeccompAction(sc.Action)
		conds := ociArgsToConditions(sc.Args, action)
		for _, name := range sc.Names {
			rule := res.Syscalls[name]
			rule.Action = action
			rule.ArgConditions = append(rule.ArgConditions, conds...)
			res.Syscalls[name] = rule
		}
	}
	return res
}

func ociArgsToConditions(args []ociSeccompArg, action SeccompAction) []ArgCondition {
	if len(args) == 0 {
		return nil
	}
	conds := make([]ArgCondition, 0, len(args))
	for _, a := range args {
		op := scmpOp(a.Op)
		if op == "" {
			continue
		}
		conds = append(conds, ArgCondition{Index: int(a.Index), Op: op, Value: a.Value, Action: action})
	}
	return conds
}

func scmpActionToSeccompAction(a string) SeccompAction {
	switch a {
	case "SCMP_ACT_ALLOW":
		return ActionAllow
	case "SCMP_ACT_ERRNO":
		return ActionErrno
	case "SCMP_ACT_KILL", "SCMP_ACT_KILL_THREAD":
		return ActionKillThread
	case "SCMP_ACT_KILL_PROCESS":
		return ActionKillProcess
	case "SCMP_ACT_TRAP":
		return ActionTrap
	case "SCMP_ACT_TRACE":
		return ActionTrace
	case "SCMP_ACT_LOG":
		return ActionLog
	case "SCMP_ACT_NOTIFY":
		return ActionNotify
	default:
		return SeccompAction(a)
	}
}

// scmpOp maps an OCI SCMP_CMP_* operator to the ArgCondition Op notation.
// SCMP_CMP_MASKED_EQ maps to "&"; its second operand is not represented.
func scmpOp(op string) string {
	switch op {
	case "SCMP_CMP_EQ":
		return "=="
	case "SCMP_CMP_NE":
		return "!="
	case "SCMP_CMP_LT":
		return "<"
	case "SCMP_CMP_LE":
		return "<="
	case "SCMP_CMP_GT":
		return ">"
	case "SCMP_CMP_GE":
		return ">="
	case "SCMP_CMP_MASKED_EQ":
		return "&"
	default:
		return ""
	}
}

const kubeletConfigPath = "/var/lib/kubelet/config.yaml"

// kubeletSeccompDefaultEnabled reports whether the node's kubelet applies the
// RuntimeDefault seccomp profile to workloads that declare none. The kubelet
// resolves this from the --seccomp-default flag (which wins) or the
// seccompDefault field in its config file. Returns false when it can't be
// determined, so callers leave the undeclared case unknown rather than guess.
func kubeletSeccompDefaultEnabled() bool {
	if v, found := kubeletFlagSeccompDefault(); found {
		return v
	}
	return kubeletConfigSeccompDefault()
}

func kubeletConfigSeccompDefault() bool {
	path := kubeletConfigPath
	if hostRoot := os.Getenv("HOST_ROOT"); hostRoot != "" {
		path = filepath.Join(hostRoot, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg struct {
		SeccompDefault *bool `yaml:"seccompDefault"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false
	}
	return cfg.SeccompDefault != nil && *cfg.SeccompDefault
}

// kubeletFlagSeccompDefault looks for a --seccomp-default flag on the running
// kubelet process. found is false when no kubelet process is located.
func kubeletFlagSeccompDefault() (val bool, found bool) {
	proc := "/proc"
	if v := os.Getenv("HOST_PROC"); v != "" {
		proc = v
	}
	entries, err := os.ReadDir(proc)
	if err != nil {
		return false, false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(proc, e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(string(data), "\x00")
		if len(args) == 0 || filepath.Base(args[0]) != "kubelet" {
			continue
		}
		for _, a := range args {
			if a == "--seccomp-default" {
				return true, true
			}
			if v, ok := strings.CutPrefix(a, "--seccomp-default="); ok {
				return v == "true" || v == "1", true
			}
		}
		return false, true // kubelet found, flag not set
	}
	return false, false
}
