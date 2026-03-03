// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package selftests holds selftests related files
package selftests

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// WindowsOpenRegistryKeyTest defines a windows open registry key self test
type WindowsOpenRegistryKeyTest struct {
	ruleID    eval.RuleID
	isSuccess bool
	keyPath   string
}

// GetRuleDefinition returns the rule
func (o *WindowsOpenRegistryKeyTest) GetRuleDefinition() *rules.RuleDefinition {
	o.ruleID = ruleIDPrefix + "_windows_open_registry_key_name"

	return &rules.RuleDefinition{
		ID:         o.ruleID,
		Expression: fmt.Sprintf(`open.registry.key_name == "%s"`, filepath.Base(o.keyPath)),
		Silent:     true,
	}
}

// GenerateEvent generate an event
func (o *WindowsOpenRegistryKeyTest) GenerateEvent(ctx context.Context) error {
	o.isSuccess = false

	cmd := exec.CommandContext(ctx,
		"powershell",
		"-c",
		"Get-ItemProperty",
		"-Path",
		o.keyPath,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error opening registry key %s: %w", o.keyPath, err)
	}

	return nil
}

// HandleEvent handles self test events
func (o *WindowsOpenRegistryKeyTest) HandleEvent(event selfTestEvent) {
	o.isSuccess = event.RuleID == o.ruleID
}

// IsSuccess return the state of the test
func (o *WindowsOpenRegistryKeyTest) IsSuccess() bool {
	return o.isSuccess
}
