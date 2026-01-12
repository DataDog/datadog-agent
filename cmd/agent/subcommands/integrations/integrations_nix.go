// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin && python

package integrations

import (
	"errors"
	"os"
)

func validateUser(allowRoot bool) error {
	if os.Geteuid() == 0 && !allowRoot {
		return errors.New("operation is disabled for root user. Please run this tool with the agent-running user or add '--allow-root/-r' to force")
	}
	return nil
}
