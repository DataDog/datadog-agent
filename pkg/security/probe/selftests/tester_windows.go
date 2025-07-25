// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package selftests

import (
	"fmt"
	"path/filepath"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/utils/pathutils"
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

	dirLongPath, err := pathutils.GetLongPathName(dir)
	if err != nil {
		return nil, err
	}

	selfTests = []SelfTest{
		&WindowsCreateFileSelfTest{filename: filepath.Join(dirLongPath, fileToCreate)},
		&WindowsOpenRegistryKeyTest{keyPath: keyPath},
	}

	s := &SelfTester{
		waitingForEvent: atomic.NewBool(false),
		eventChan:       make(chan selfTestEvent, 10),
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
