// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package selftests

import (
	"fmt"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OpenSelfTest defines an open self test
type OpenSelfTest struct {
}

// GetRuleDefinition returns the rule
func (o *OpenSelfTest) GetRuleDefinition(filename string) *rules.RuleDefinition {
	return &rules.RuleDefinition{
		ID:         fmt.Sprintf("%s_open", ruleIDPrefix),
		Expression: fmt.Sprintf(`open.file.path == "%s"`, filename),
	}
}

// GenerateEvent generate an event
func (o *OpenSelfTest) GenerateEvent(filename string) (EventPredicate, error) {
	// we need to use touch (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	cmd := exec.Command("touch", filename)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running touch: %v", err)
		return nil, err
	}

	return func(event selfTestEvent) bool {
		return event.Type == "open" && event.Filepath == filename
	}, nil
}
