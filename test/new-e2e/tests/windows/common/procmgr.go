// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// ProcmgrDescribe runs `dd-procmgr describe` for processName using the CLI at cliPath and
// returns its fields keyed by label, along with the raw output for failure messages.
//
// Callers that need more than one field should use this rather than calling
// ProcmgrDescribeField twice. Two describes can straddle a restart and report a State and a
// PID that never belonged to the same process.
func ProcmgrDescribe(host *components.RemoteHost, cliPath, processName string) (map[string]string, string, error) {
	cmd := fmt.Sprintf(`& '%s' describe %s`, strings.ReplaceAll(cliPath, `'`, `''`), processName)
	out, err := host.Execute(cmd)
	out = strings.TrimSpace(out)
	if err != nil {
		return nil, out, fmt.Errorf("dd-procmgr describe %s failed: %w, output: %s", processName, err, out)
	}
	fields := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		// Cut on the first colon: values such as Command carry their own (C:\...).
		label, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		fields[label] = strings.TrimSpace(value)
	}
	return fields, out, nil
}

// ProcmgrDescribeField runs `dd-procmgr describe` for processName using the CLI at cliPath and
// returns the value of field.
func ProcmgrDescribeField(host *components.RemoteHost, cliPath, processName, field string) (string, error) {
	fields, out, err := ProcmgrDescribe(host, cliPath, processName)
	if err != nil {
		return "", err
	}
	value, ok := fields[field]
	if !ok {
		return "", fmt.Errorf("field %q not found in dd-procmgr describe %s output: %s", field, processName, out)
	}
	return value, nil
}
