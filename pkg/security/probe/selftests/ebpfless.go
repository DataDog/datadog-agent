// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package selftests holds selftests related files
package selftests

import (
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// EBPFLessSelfTest defines an ebpf less self test
type EBPFLessSelfTest struct {
	ruleID    eval.RuleID
	isSuccess bool
}

// GetRuleDefinition returns the rule
func (o *EBPFLessSelfTest) GetRuleDefinition() *rules.RuleDefinition {
	o.ruleID = fmt.Sprintf("%s_exec", ruleIDPrefix)

	return &rules.RuleDefinition{
		ID:         o.ruleID,
		Expression: `exec.file.path != "" && process.parent.pid == 0 && process.ppid == 0`,
		Every:      time.Duration(math.MaxInt64),
		Silent:     true,
	}
}

// GenerateEvent generate an event
func (o *EBPFLessSelfTest) GenerateEvent() error {
	return nil
}

// HandleEvent handles self test events
func (o *EBPFLessSelfTest) HandleEvent(event selfTestEvent) {
	o.isSuccess = event.RuleID == o.ruleID
}

// IsSuccess return the state of the test
func (o *EBPFLessSelfTest) IsSuccess() bool {
	return o.isSuccess
}
