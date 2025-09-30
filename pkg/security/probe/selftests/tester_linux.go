// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package selftests

import (
	"os"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(cfg *config.RuntimeSecurityConfig, probe *probe.Probe) (*SelfTester, error) {
	var (
		selfTests []SelfTest
		tmpDir    string
	)

	if cfg.EBPFLessEnabled {
		selfTests = []SelfTest{
			&EBPFLessSelfTest{},
		}
	} else {
		name, dir, err := createTargetFile()
		if err != nil {
			return nil, err
		}
		tmpDir = dir

		selfTests = []SelfTest{
			&OpenSelfTest{filename: name},
			&ChmodSelfTest{filename: name},
			&ChownSelfTest{filename: name},
		}
	}

	s := &SelfTester{
		waitingForEvent: atomic.NewBool(cfg.EBPFLessEnabled),
		eventChan:       make(chan selfTestEvent, len(selfTests)),
		selfTestRunning: make(chan time.Duration, 10),
		probe:           probe,
		selfTests:       selfTests,
		tmpDir:          tmpDir,
		done:            make(chan bool),
		config:          cfg,
		errorTimestamp:  make(map[eval.RuleID]time.Time),
	}

	return s, nil
}

func createTargetFile() (string, string, error) {
	// Create temp directory to put target file in
	tmpDir, err := os.MkdirTemp("", "datadog_agent_cws_self_test")
	if err != nil {
		return "", "", err
	}

	// Create target file
	targetFile, err := os.CreateTemp(tmpDir, "datadog_agent_cws_target_file")
	if err != nil {
		return "", "", err
	}

	return targetFile.Name(), tmpDir, targetFile.Close()
}
