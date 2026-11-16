// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securitycontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
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

// defaultKubeletRootDir is the kubelet --root-dir default.
const defaultKubeletRootDir = "/var/lib/kubelet"

// kubeletInfo holds the flags of the running kubelet.
type kubeletInfo struct {
	pid            string
	rootDir        string
	configPath     string
	seccompDefault *bool
}

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
func readLocalhostSeccompProfile(kubelet *kubeletInfo, localhostPath string) (*SeccompFilterResult, error) {
	if localhostPath == "" {
		return nil, errors.New("empty localhost profile path")
	}
	rootDir := defaultKubeletRootDir
	if kubelet != nil {
		rootDir = kubelet.rootDir
	}
	path, data, err := readHostFile(kubelet, filepath.Join(rootDir, "seccomp", localhostPath))
	if err != nil {
		return nil, fmt.Errorf("read seccomp profile %s: %w", localhostPath, err)
	}
	var prof ociSeccompProfile
	if err := json.Unmarshal(data, &prof); err != nil {
		return nil, fmt.Errorf("parse seccomp profile %s: %w", path, err)
	}
	return ociToFilterResult(&prof), nil
}

// hostRoots returns the prefixes under which a host path may be reachable,
// most reliable first. The kubelet's root comes first since it may itself run
// in a container (e.g. kind nodes).
func hostRoots(kubelet *kubeletInfo) []string {
	var roots []string
	if kubelet != nil && kubelet.pid != "" {
		roots = append(roots, filepath.Join(procDir(), kubelet.pid, "root"))
	}
	if hostRoot := os.Getenv("HOST_ROOT"); hostRoot != "" {
		roots = append(roots, hostRoot)
	}
	return append(roots, filepath.Join(procDir(), "1", "root"), "/")
}

// readHostFile reads hostPath under the first host root where it is readable.
func readHostFile(kubelet *kubeletInfo, hostPath string) (string, []byte, error) {
	roots := hostRoots(kubelet)
	var firstErr error
	for _, root := range roots {
		path := filepath.Join(root, hostPath)
		data, err := os.ReadFile(path)
		if err == nil {
			return path, data, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return "", nil, fmt.Errorf("%s not readable under host roots %v: %w", hostPath, roots, firstErr)
}

func procDir() string {
	if v := os.Getenv("HOST_PROC"); v != "" {
		return v
	}
	return "/proc"
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

// kubeletSeccompDefaultEnabled reports whether the node's kubelet applies the
// RuntimeDefault seccomp profile to workloads that declare none. The kubelet
// resolves this from the --seccomp-default flag (which wins) or the
// seccompDefault field in its config file. Returns false when it can't be
// determined, so callers leave the undeclared case unknown rather than guess.
func kubeletSeccompDefaultEnabled(kubelet *kubeletInfo) bool {
	if kubelet != nil && kubelet.seccompDefault != nil {
		return *kubelet.seccompDefault
	}
	return kubeletConfigSeccompDefault(kubelet)
}

func kubeletConfigSeccompDefault(kubelet *kubeletInfo) bool {
	path := filepath.Join(defaultKubeletRootDir, "config.yaml")
	if kubelet != nil && kubelet.configPath != "" {
		path = kubelet.configPath
	}
	_, data, err := readHostFile(kubelet, path)
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

// findKubelet returns the running kubelet's flags, or nil if none is found.
func findKubelet() *kubeletInfo {
	proc := procDir()
	entries, err := os.ReadDir(proc)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if args, ok := kubeletArgs(e.Name()); ok {
			return parseKubeletArgs(e.Name(), args)
		}
	}
	return nil
}

// kubeletArgs returns the arguments of pid if it is a kubelet process.
func kubeletArgs(pid string) ([]string, bool) {
	data, err := os.ReadFile(filepath.Join(procDir(), pid, "cmdline"))
	if err != nil {
		return nil, false
	}
	args := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	if filepath.Base(args[0]) != "kubelet" {
		return nil, false
	}
	return args[1:], true
}

func parseKubeletArgs(pid string, args []string) *kubeletInfo {
	info := &kubeletInfo{pid: pid, rootDir: defaultKubeletRootDir}
	for i, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		switch name {
		case "--seccomp-default":
			v := !hasValue || value == "true" || value == "1"
			info.seccompDefault = &v
		case "--root-dir", "--config":
			if !hasValue {
				if i+1 >= len(args) {
					continue
				}
				value = args[i+1]
			}
			if value == "" {
				continue
			}
			if name == "--root-dir" {
				info.rootDir = value
			} else {
				info.configPath = value
			}
		}
	}
	return info
}
