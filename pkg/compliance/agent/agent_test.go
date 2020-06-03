// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package agent

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type tempEnv struct {
	dir  string
	prev string
}

func (e *tempEnv) leave() {
	defer os.Chdir(e.prev)
}

func enterTempEnv(t *testing.T) *tempEnv {
	t.Helper()
	assert := assert.New(t)
	tempDir, err := ioutil.TempDir("", "compliance-agent-")
	assert.NoError(err)

	err = util.CopyDir("./testdata/configs", tempDir)
	assert.NoError(err)

	files, err := filepath.Glob(filepath.Join(tempDir, "files/*"))
	assert.NoError(err)
	for _, file := range files {
		_ = os.Chmod(file, 0644)
	}

	prev, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	return &tempEnv{
		dir:  tempDir,
		prev: prev,
	}
}

func TestRun(t *testing.T) {
	assert := assert.New(t)

	e := enterTempEnv(t)
	defer e.leave()

	reporter := &mocks.Reporter{}

	reporter.On("Report", &compliance.RuleEvent{
		RuleID:       "cis-docker-1",
		Framework:    "cis-docker",
		Version:      "1.2.0",
		ResourceID:   "the-host",
		ResourceType: "docker",
		Tags:         []string{"check_kind:file"},
		Data: compliance.KVMap{
			"permissions": "644",
		},
	})

	reporter.On("Report", &compliance.RuleEvent{
		RuleID:       "cis-kubernetes-1",
		Framework:    "cis-kubernetes",
		Version:      "1.5.0",
		ResourceID:   "the-host",
		ResourceType: "worker",
		Tags:         []string{"check_kind:file"},
		Data: compliance.KVMap{
			"permissions": "644",
		},
	})
	defer reporter.AssertExpectations(t)

	scheduler := &mocks.Scheduler{}
	defer scheduler.AssertExpectations(t)

	scheduler.On("Run").Once().Return(nil)
	scheduler.On("Stop").Once().Return(nil)

	scheduler.On("Enter", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		check := args.Get(0).(check.Check)
		check.Run()
	})

	agent, err := New(reporter, scheduler, e.dir, checks.WithHostname("the-host"))
	assert.NoError(err)

	err = agent.Run()
	assert.NoError(err)
	agent.Stop()
}

func TestRunChecks(t *testing.T) {
	assert := assert.New(t)

	e := enterTempEnv(t)
	defer e.leave()

	reporter := &mocks.Reporter{}

	reporter.On("Report", &compliance.RuleEvent{
		RuleID:       "cis-docker-1",
		Framework:    "cis-docker",
		Version:      "1.2.0",
		ResourceID:   "the-host",
		ResourceType: "docker",
		Tags:         []string{"check_kind:file"},
		Data: compliance.KVMap{
			"permissions": "644",
		},
	})

	defer reporter.AssertExpectations(t)

	err := RunChecks(
		reporter,
		e.dir,
		checks.WithMatchSuite(checks.IsFramework("cis-docker")),
		checks.WithMatchRule(checks.IsRuleID("cis-docker-1")),
		checks.WithHostname("the-host"),
	)
	assert.NoError(err)
}

func TestRunChecksFromFile(t *testing.T) {
	assert := assert.New(t)
	e := enterTempEnv(t)
	defer e.leave()

	reporter := &mocks.Reporter{}

	reporter.On("Report", &compliance.RuleEvent{
		RuleID:       "cis-kubernetes-1",
		Framework:    "cis-kubernetes",
		Version:      "1.5.0",
		ResourceID:   "the-host",
		ResourceType: "worker",
		Tags:         []string{"check_kind:file"},
		Data: compliance.KVMap{
			"permissions": "644",
		},
	})

	defer reporter.AssertExpectations(t)

	err := RunChecksFromFile(
		reporter,
		filepath.Join(e.dir, "cis-kubernetes.yaml"),
		checks.WithHostname("the-host"),
	)
	assert.NoError(err)
}
