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

// ChmodSelfTest defines a chmod self test
type ChmodSelfTest struct{}

// GetRuleDefinition returns the rule
func (o *ChmodSelfTest) GetRuleDefinition(filename string) *rules.RuleDefinition {
	return &rules.RuleDefinition{
		ID:         fmt.Sprintf("%s_chmod", ruleIDPrefix),
		Expression: fmt.Sprintf(`chmod.file.path == "%s"`, filename),
	}
}

// GenerateEvent generate an event
func (o *ChmodSelfTest) GenerateEvent(filename string) (EventPredicate, error) {
	// we need to use chmod (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	cmd := exec.Command("chmod", "777", filename)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running chmod: %v", err)
		return nil, err
	}

	return func(event selfTestEvent) bool {
		return event.Type == "chmod" && event.Filepath == filename
	}, nil
}
