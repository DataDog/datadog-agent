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

// ProcmgrDescribeField runs `dd-procmgr describe` for processName using the CLI at cliPath and
// returns the value of field.
func ProcmgrDescribeField(host *components.RemoteHost, cliPath, processName, field string) (string, error) {
	cmd := fmt.Sprintf(`& '%s' describe %s`, strings.ReplaceAll(cliPath, `'`, `''`), processName)
	out, err := host.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("dd-procmgr describe %s failed: %w, output: %s", processName, err, strings.TrimSpace(out))
	}
	for _, line := range strings.Split(out, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), field+":"); ok {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("field %q not found in dd-procmgr describe %s output: %s", field, processName, strings.TrimSpace(out))
}
