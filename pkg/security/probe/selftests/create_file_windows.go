// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package selftests

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// WindowsCreateFileSelfTest defines a windows create file self test
type WindowsCreateFileSelfTest struct {
	ruleID    eval.RuleID
	isSuccess bool
	filename  string
}

// GetRuleDefinition returns the rule
func (o *WindowsCreateFileSelfTest) GetRuleDefinition() *rules.RuleDefinition {
	o.ruleID = fmt.Sprintf("%s_windows_create_file", ruleIDPrefix)

	basename := filepath.Base(o.filename)
	devicePath := o.filename
	volumeName := filepath.VolumeName(o.filename)
	// replace volume name with glob matching the device name
	if volumeName != "" {
		devicePath = "/Device/*" + o.filename[len(volumeName):]
	}

	return &rules.RuleDefinition{
		ID:         o.ruleID,
		Expression: fmt.Sprintf(`create.file.name == "%s" && create.file.device_path =~ "%s" && process.pid == %d`, basename, filepath.ToSlash(devicePath), os.Getpid()),
		Silent:     true,
	}
}

// GenerateEvent generate an event
func (o *WindowsCreateFileSelfTest) GenerateEvent() error {
	o.isSuccess = false

	file, err := os.Create(o.filename)
	if err != nil {
		log.Debugf("error creating file: %v", err)
		return err
	}
	return file.Close()
}

// HandleEvent handles self test events
func (o *WindowsCreateFileSelfTest) HandleEvent(event selfTestEvent) {
	o.isSuccess = event.RuleID == o.ruleID
}

// IsSuccess return the state of the test
func (o *WindowsCreateFileSelfTest) IsSuccess() bool {
	return o.isSuccess
}
