// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package selftests holds selftests related files
package selftests

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows/registry"
)

// WindowsSetRegistryKeyTest defines a windows set registry value self test
type WindowsSetRegistryKeyTest struct {
	ruleID    eval.RuleID
	isSuccess bool
	keyName   string
}

// GetRuleDefinition returns the rule
func (o *WindowsSetRegistryKeyTest) GetRuleDefinition() *rules.RuleDefinition {
	o.ruleID = fmt.Sprintf("%s_windows_open_registry_key_value", ruleIDPrefix)

	return &rules.RuleDefinition{
		ID:         o.ruleID,
		Expression: fmt.Sprintf(`set.registry.key_name == "%s"`, filepath.Base(o.keyName)),
	}
}

// GenerateEvent generate an event
func (o *WindowsSetRegistryKeyTest) GenerateEvent() error {
	o.isSuccess = false

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, o.keyName, registry.SET_VALUE)
	if err != nil {
		log.Debugf("error opening registry key: %v", err)
		return err
	}
	defer key.Close()

	if err := key.SetStringValue("tmp_self_test_value_name", "tmp_self_test_value"); err != nil {
		log.Debugf("error setting registry value: %v", err)
		return err
	}

	o.isSuccess = true
	return nil
}

// HandleEvent handles self test events
func (o *WindowsSetRegistryKeyTest) HandleEvent(event selfTestEvent) {
	o.isSuccess = event.RuleID == o.ruleID
}

// IsSuccess return the state of the test
func (o *WindowsSetRegistryKeyTest) IsSuccess() bool {
	return o.isSuccess
}
