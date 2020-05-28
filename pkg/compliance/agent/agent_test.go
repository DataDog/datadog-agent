// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRun(t *testing.T) {
	assert := assert.New(t)

	tempDir, err := ioutil.TempDir("", "compliance-agent-")
	assert.NoError(err)

	err = copyDir("./testdata/configs", tempDir)
	assert.NoError(err)

	files, err := filepath.Glob(filepath.Join(tempDir, "files/*"))
	assert.NoError(err)
	for _, file := range files {
		_ = os.Chmod(file, 0644)
	}

	prev, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(prev)

	reporter := &mocks.Reporter{}

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

	scheduler := &mocks.Scheduler{}
	defer scheduler.AssertExpectations(t)

	scheduler.On("Run").Once().Return(nil)
	scheduler.On("Stop").Once().Return(nil)

	scheduler.On("Enter", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		check := args.Get(0).(check.Check)
		check.Run()
	})

	a := New(reporter, scheduler, tempDir, checks.WithHostname("the-host"), checks.WithDockerClient(&mocks.DockerClient{}))

	err = a.Run()
	assert.NoError(err)
	a.Stop()
}

func copyFile(src, dst string) error {
	var (
		err  error
		s, d *os.File
	)

	if s, err = os.Open(src); err != nil {
		return err
	}
	defer s.Close()

	if d, err = os.Create(dst); err != nil {
		return err
	}
	defer d.Close()

	if _, err = io.Copy(d, s); err != nil {
		return err
	}
	return nil
}

func copyDir(src, dst string) error {
	var (
		err     error
		fds     []os.FileInfo
		srcinfo os.FileInfo
	)

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		s := path.Join(src, fd.Name())
		d := path.Join(dst, fd.Name())

		if fd.IsDir() {
			err = copyDir(s, d)
		} else {
			err = copyFile(s, d)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
