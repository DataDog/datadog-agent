// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"sort"
	"strings"
)

// procmgrCreateSpec builds a `dd-procmgr create` argument list. Values that look
// like flags (e.g. PowerShell -NoProfile) use `--args=<value>` so they survive
// both clap parsing and remote shell quoting.
type procmgrCreateSpec struct {
	Name          string
	Command       string
	Args          []string
	Env           map[string]string
	RestartPolicy string
	Description   string
	NoAutoStart   bool
}

func (s *baseProcmgrSuite) cliList() string {
	return s.platform.cliCmd("list")
}

func (s *baseProcmgrSuite) cliStatus() string {
	return s.platform.cliCmd("status")
}

func (s *baseProcmgrSuite) cliDescribe(nameOrUUID string) string {
	return s.platform.cliCmd("describe " + quoteCLIWord(nameOrUUID))
}

func (s *baseProcmgrSuite) cliStart(nameOrUUID string) string {
	return s.platform.cliCmd("start " + quoteCLIWord(nameOrUUID))
}

func (s *baseProcmgrSuite) cliStop(nameOrUUID string) string {
	return s.platform.cliCmd("stop " + quoteCLIWord(nameOrUUID))
}

func (s *baseProcmgrSuite) cliCreate(spec procmgrCreateSpec) string {
	return s.platform.cliCmd(spec.cliArgs())
}

func (spec procmgrCreateSpec) cliArgs() string {
	parts := []string{
		"create",
		formatCLIFlag("name", spec.Name),
		formatCLIFlag("command", spec.Command),
	}
	if len(spec.Args) > 0 {
		parts = append(parts, formatCLIRepeatableFlag("args", spec.Args))
	}
	if len(spec.Env) > 0 {
		keys := make([]string, 0, len(spec.Env))
		for key := range spec.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, formatCLIFlag("env", key+"="+spec.Env[key]))
		}
	}
	if spec.RestartPolicy != "" {
		parts = append(parts, formatCLIFlag("restart-policy", spec.RestartPolicy))
	}
	if spec.Description != "" {
		parts = append(parts, formatCLIFlag("description", spec.Description))
	}
	if spec.NoAutoStart {
		parts = append(parts, "--no-auto-start")
	}
	return strings.Join(parts, " ")
}

func formatCLIFlag(name, value string) string {
	if value == "" {
		return "--" + name
	}
	v := value
	if needsCLIQuoting(value) {
		if strings.HasPrefix(value, "-") && !hasShellMetachar(value[1:]) {
			v = value
		} else {
			v = quoteCLIValue(value)
		}
	}
	return "--" + name + "=" + v
}

func formatCLIRepeatableFlag(name string, values []string) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = formatCLIFlag(name, value)
	}
	return strings.Join(parts, " ")
}

func quoteCLIWord(value string) string {
	if !needsCLIQuoting(value) {
		return value
	}
	return quoteCLIValue(value)
}

func quoteCLIValue(value string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(value, `'`, `''`))
}

func needsCLIQuoting(value string) bool {
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "-") {
		return true
	}
	return hasShellMetachar(value)
}

func hasShellMetachar(value string) bool {
	return strings.ContainsAny(value, " \t'\"\\;|&<>()$`")
}
