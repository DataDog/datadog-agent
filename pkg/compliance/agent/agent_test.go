// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/containerd/continuity/fs"
)

func TestRun(t *testing.T) {

	dockerClient = func() checks.DockerClient {
		return &checks.MockDockerClient{}
	}

	assert := assert.New(t)

	tempDir, err := ioutil.TempDir("", "compliance-agent-*")
	assert.NoError(err)

	err = fs.CopyDir(tempDir, "./testdata/configs")
	assert.NoError(err)

	files, err := filepath.Glob(filepath.Join(tempDir, "files/*"))
	assert.NoError(err)
	for _, file := range files {
		_ = os.Chmod(file, 0644)
	}

	prev, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(prev)

	interval := time.Hour

	reporter := &compliance.MockReporter{}

	reporter.On("Report", &compliance.RuleEvent{
		RuleID:       "cis-docker-1",
		Framework:    "cis-docker",
		Version:      "1.2.0",
		ResourceID:   "the-host",
		ResourceType: "docker",
		Data: compliance.KV{
			"permissions": "644",
		},
	})

	reporter.On("Report", &compliance.RuleEvent{
		RuleID:       "cis-kubernetes-1",
		Framework:    "cis-kubernetes",
		Version:      "1.5.0",
		ResourceID:   "the-host",
		ResourceType: "worker",
		Data: compliance.KV{
			"permissions": "644",
		},
	})
	defer reporter.AssertExpectations(t)

	scheduler := &MockScheduler{}
	defer scheduler.AssertExpectations(t)

	scheduler.On("Run").Once().Return(nil)
	scheduler.On("Stop").Once().Return(nil)

	scheduler.On("Enter", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		check := args.Get(0).(check.Check)
		check.Run()
	})

	a := New(reporter, scheduler, tempDir, "the-host", interval)

	err = a.Run()
	assert.NoError(err)
	a.Stop()
}
