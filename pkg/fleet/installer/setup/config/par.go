// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// PARInstallerDefault returns the supported installer's default, or nil when
// the platform is unsupported or an operator already configured the runner.
// Windows starts the process manager through SCM; Linux requires live systemd.
func PARInstallerDefault(path, goos string, procmgr, fips bool, systemdRunning func() (bool, error)) (*bool, error) {
	if !procmgr || fips || (goos != "linux" && goos != "windows") {
		return nil, nil
	}
	if goos == "linux" {
		running, err := systemdRunning()
		if err != nil {
			return nil, fmt.Errorf("check systemd support: %w", err)
		}
		if !running {
			return nil, nil
		}
	}
	switch strings.ToLower(os.Getenv("DD_PRIVATE_ACTION_RUNNER_ENABLED")) {
	case "false", "0":
		return BoolToPtr(false), nil
	case "true", "1":
		return BoolToPtr(true), nil
	case "":
	default:
		return nil, nil
	}
	data, err := readConfig(path)
	if err != nil {
		return nil, fmt.Errorf("read existing Agent configuration: %w", err)
	}
	var current struct {
		PrivateActionRunner map[string]any `yaml:"private_action_runner"`
	}
	if err := yaml.Unmarshal(data, &current); err != nil {
		return nil, fmt.Errorf("read existing PAR setting: %w", err)
	}
	if _, configured := current.PrivateActionRunner["enabled"]; configured {
		return nil, nil
	}
	return BoolToPtr(true), nil
}
