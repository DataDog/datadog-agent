// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package selftests

import (
	"fmt"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(cfg *config.RuntimeSecurityConfig, probe *probe.Probe) (*SelfTester, error) {

	if !cfg.FIMEnabled {
		return nil, fmt.Errorf("FIM is disabled")
	}
	var (
		selfTests []SelfTest
		tmpDir    string
	)

	dir, err := CreateTargetDir()
	if err != nil {
		return nil, err
	}
	tmpDir = dir
	fileToCreate := "file.txt"

	keyPath := "Software\\Datadog\\Datadog Agent"
	if err != nil {
		return nil, err
	}
	selfTests = []SelfTest{
		&WindowsCreateFileSelfTest{filename: fmt.Sprintf("%s/%s", dir, fileToCreate)},
		&WindowsOpenRegistryKeyTest{keyPath: keyPath},
	}

	s := &SelfTester{
		waitingForEvent: atomic.NewBool(cfg.EBPFLessEnabled),
		eventChan:       make(chan selfTestEvent, 10),
		selfTestRunning: make(chan time.Duration, 10),
		probe:           probe,
		selfTests:       selfTests,
		tmpDir:          tmpDir,
		done:            make(chan bool),
		config:          cfg,
	}

	return s, nil
}
