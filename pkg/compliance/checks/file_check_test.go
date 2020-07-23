// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package checks

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
)

func TestFileCheck(t *testing.T) {
	type setupFileFunc func(t *testing.T, env *mocks.Env, file *compliance.File)
	type validateFunc func(t *testing.T, file *compliance.File, report *report)

	normalizePath := func(t *testing.T, env *mocks.Env, file *compliance.File) {
		t.Helper()
		env.On("NormalizeToHostRoot", file.Path).Return(file.Path)
		env.On("RelativeToHostRoot", file.Path).Return(file.Path)
	}

	cleanUpDirs := make([]string, 0)
	createTempFiles := func(t *testing.T, numFiles int) (string, []string) {
		paths := make([]string, 0, numFiles)
		dir, err := ioutil.TempDir("", "cmplFileTest")
		assert.NoError(t, err)
		cleanUpDirs = append(cleanUpDirs, dir)

		for i := 0; i < numFiles; i++ {
			fileName := fmt.Sprintf("test-%d-%d.dat", i, time.Now().Unix())
			filePath := path.Join(dir, fileName)
			paths = append(paths, filePath)

			f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
			defer f.Close()
			assert.NoError(t, err)
		}

		return dir, paths
	}

	tests := []struct {
		name        string
		resource    compliance.Resource
		setup       setupFileFunc
		validate    validateFunc
		expectError error
	}{
		{
			name: "file permissions",
			resource: compliance.Resource{
				File: &compliance.File{
					Path: "/etc/test-permissions.dat",
				},
				Condition: "file.permissions == 0644",
			},
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				_, filePaths := createTempFiles(t, 1)

				env.On("NormalizeToHostRoot", file.Path).Return(filePaths[0])
				env.On("RelativeToHostRoot", filePaths[0]).Return(file.Path)
			},
			validate: func(t *testing.T, file *compliance.File, report *report) {
				assert.True(t, report.passed)
				assert.Equal(t, file.Path, report.data["file.path"])
				assert.Equal(t, uint64(0644), report.data["file.permissions"])
			},
		},
		{
			name: "file permissions (glob)",
			resource: compliance.Resource{
				File: &compliance.File{
					Path: "/etc/*.dat",
				},
				Condition: "file.permissions == 0644",
			},
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				tempDir, filePaths := createTempFiles(t, 2)
				for _, filePath := range filePaths {
					env.On("RelativeToHostRoot", filePath).Return(path.Join("/etc/", path.Base(filePath)))
				}

				env.On("NormalizeToHostRoot", file.Path).Return(path.Join(tempDir, "/*.dat"))
			},
			validate: func(t *testing.T, file *compliance.File, report *report) {
				assert.True(t, report.passed)
				assert.Regexp(t, "/etc/test-[0-9]-[0-9]+", report.data["file.path"])
				assert.Equal(t, uint64(0644), report.data["file.permissions"])
			},
		},
		{
			name: "file user and group",
			resource: compliance.Resource{
				File: &compliance.File{
					Path: "/tmp",
				},
				Condition: `file.user == "root" && file.group in ["root", "wheel"]`,
			},
			setup: normalizePath,
			validate: func(t *testing.T, file *compliance.File, report *report) {
				assert.True(t, report.passed)
				assert.Equal(t, "/tmp", report.data["file.path"])
				assert.Equal(t, "root", report.data["file.user"])
				assert.Contains(t, []string{"root", "wheel"}, report.data["file.group"])
			},
		},
		{
			name: "jq(log-driver) - passed",
			resource: compliance.Resource{
				File: &compliance.File{
					Path: "/etc/docker/daemon.json",
				},
				Condition: `file.jq(".\"log-driver\"") == "json-file"`,
			},
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/file/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/file/daemon.json").Return(file.Path)
			},
			validate: func(t *testing.T, file *compliance.File, report *report) {
				assert.True(t, report.passed)
				assert.Equal(t, "/etc/docker/daemon.json", report.data["file.path"])
				assert.NotEmpty(t, report.data["file.user"])
				assert.NotEmpty(t, report.data["file.group"])
			},
		},
		{
			name: "jq(experimental) - failed",
			resource: compliance.Resource{
				File: &compliance.File{
					Path: "/etc/docker/daemon.json",
				},
				Condition: `file.jq(".experimental") == "true"`,
			},
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/file/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/file/daemon.json").Return(file.Path)
			},
			validate: func(t *testing.T, file *compliance.File, report *report) {
				assert.False(t, report.passed)
				assert.Equal(t, "/etc/docker/daemon.json", report.data["file.path"])
				assert.NotEmpty(t, report.data["file.user"])
				assert.NotEmpty(t, report.data["file.group"])
			},
		},
		{
			name: "jq(experimental) and path expression",
			resource: compliance.Resource{
				File: &compliance.File{
					Path: `process.flag("dockerd", "--config-file")`,
				},
				Condition: `file.jq(".experimental") == "false"`,
			},
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				path := "/etc/docker/daemon.json"
				env.On("EvaluateFromCache", mock.Anything).Return(path, nil)
				env.On("NormalizeToHostRoot", path).Return("./testdata/file/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/file/daemon.json").Return(path)
			},
			validate: func(t *testing.T, file *compliance.File, report *report) {
				assert.True(t, report.passed)
				assert.Equal(t, "/etc/docker/daemon.json", report.data["file.path"])
				assert.NotEmpty(t, report.data["file.user"])
				assert.NotEmpty(t, report.data["file.group"])
			},
		},
		{
			name: "jq(ulimits)",
			resource: compliance.Resource{
				File: &compliance.File{
					Path: "/etc/docker/daemon.json",
				},
				Condition: `file.jq(".[\"default-ulimits\"].nofile.Hard") == "64000"`,
			},
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/file/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/file/daemon.json").Return(file.Path)
			},
			validate: func(t *testing.T, file *compliance.File, report *report) {
				assert.True(t, report.passed)
				assert.Equal(t, "/etc/docker/daemon.json", report.data["file.path"])
				assert.NotEmpty(t, report.data["file.user"])
				assert.NotEmpty(t, report.data["file.group"])
			},
		},
		{
			name: "yaml(apiVersion)",
			resource: compliance.Resource{
				File: &compliance.File{
					Path: "/etc/pod.yaml",
				},
				Condition: `file.yaml(".apiVersion") == "v1"`,
			},
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/file/pod.yaml")
				env.On("RelativeToHostRoot", "./testdata/file/pod.yaml").Return(file.Path)
			},
			validate: func(t *testing.T, file *compliance.File, report *report) {
				assert.True(t, report.passed)
				assert.Equal(t, "/etc/pod.yaml", report.data["file.path"])
				assert.NotEmpty(t, report.data["file.user"])
				assert.NotEmpty(t, report.data["file.group"])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := &mocks.Env{}
			defer env.AssertExpectations(t)

			if test.setup != nil {
				test.setup(t, env, test.resource.File)
			}

			expr, err := eval.ParseIterable(test.resource.Condition)
			assert.NoError(t, err)

			report, err := checkFile(env, "rule-id", test.resource, expr)

			if test.expectError != nil {
				assert.Equal(t, test.expectError, err)
			} else {
				assert.NoError(t, err)
				test.validate(t, test.resource.File, report)
			}
		})
	}

	for _, dir := range cleanUpDirs {
		os.RemoveAll(dir)
	}
}
