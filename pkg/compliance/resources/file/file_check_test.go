// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/compliance/rego"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/process"
	resource_test "github.com/DataDog/datadog-agent/pkg/compliance/resources/tests"
	processutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/process"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
)

func setDefaultHooks(env *mocks.Env) {
	env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
	env.On("DumpInputPath").Return("").Maybe()
	env.On("ShouldSkipRegoEval").Return(false).Maybe()
	env.On("Hostname").Return("test-host").Maybe()
}

func normalizePath(t *testing.T, env *mocks.Env, file *compliance.File) {
	t.Helper()
	env.On("MaxEventsPerRun").Return(30).Maybe()
	env.On("NormalizeToHostRoot", file.Path).Return(file.Path)
	env.On("RelativeToHostRoot", file.Path).Return(file.Path)
	setDefaultHooks(env)
}

func createTempFiles(t *testing.T, numFiles int) (string, []string) {
	paths := make([]string, 0, numFiles)
	dir := t.TempDir()

	for i := 0; i < numFiles; i++ {
		fileName := fmt.Sprintf("test-%d-%d.dat", i, time.Now().Unix())
		filePath := path.Join(dir, fileName)
		paths = append(paths, filePath)

		f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		assert.NoError(t, err)
		defer f.Close()
	}

	return dir, paths
}

func TestFileCheck(t *testing.T) {
	assert := assert.New(t)

	type setupFileFunc func(t *testing.T, env *mocks.Env, file *compliance.File)
	type validateFunc func(t *testing.T, file *compliance.File, report *compliance.Report)

	objectModule := `package datadog

	import data.datadog as dd
	import data.helpers as h

	valid_file(file) {
		%s
	}

	max_permissions(file, consts) {
			file.permissions == bits.and(file.permissions, parse_octal(consts.max_permissions))
	}
	
	findings[f] {
			max_permissions(input.file, input.constants)
			valid_file(input.file)
			f := dd.passed_finding(
					h.resource_type,
					h.resource_id,
					h.file_data(input.file),
			)
	}
	
	findings[f] {
			not max_permissions(input.file, input.constants)
			valid_file(input.file)
			f := dd.failing_finding(
					h.resource_type,
					h.resource_id,
					h.file_data(input.file),
			)
	}

	findings[f] {
			count(input.file) == 0
			f := dd.error_finding(
					h.resource_type,
					h.resource_id,
					sprintf("no files found for file check \"%%s\"", [input.context.input.file.file.path]),
			)
	}
	
	findings[f] {
			not h.has_key(input, "file")
			f := dd.error_finding(
					h.resource_type,
					h.resource_id,
					sprintf("failed to resolve path: empty path from %%s", [input.context.input.file.file.path]),
			)
	}

	findings[f] {
		not valid_file(input.file)
		f := dd.failing_finding(
			h.resource_type,
			h.resource_id,
			h.file_data(input.file),
		)
	}
`

	arrayModule := `package datadog

	import data.datadog as dd
	import data.helpers as h

	max_permissions(file, consts) {
			file.permissions == bits.and(file.permissions, parse_octal(consts.max_permissions))
	}

	valid_file(file) {
		%s
	}

	findings[f] {
			file := input.file[_]
			valid_file(file)
			max_permissions(file, input.constants)
			f := dd.passed_finding(
					h.resource_type,
					h.resource_id,
					h.file_data(file),
			)
	}

	findings[f] {
			file := input.file[_]
			valid_file(file)
			not max_permissions(file, input.constants)
			f := dd.failing_finding(
					h.resource_type,
					h.resource_id,
					h.file_data(file),
			)
	}

	findings[f] {
			count(input.file) == 0
			f := dd.error_finding(
					h.resource_type,
					h.resource_id,
					sprintf("no files found for file check \"%%s\"", [input.context.input.file.file.path]),
			)
	}

	findings[f] {
			not h.has_key(input, "file")
			f := dd.error_finding(
					h.resource_type,
					h.resource_id,
					sprintf("failed to resolve path: empty path from %%s", [input.context.input.file.file.path]),
			)
	}
	`

	tests := []struct {
		name                string
		module              string
		additionalResources []compliance.RegoInput
		resource            compliance.RegoInput
		setup               setupFileFunc
		validate            validateFunc
		maxPermissions      string
		expectError         error
		processes           processutils.Processes
	}{
		{
			name: "file permissions",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path: "/etc/test-permissions.dat",
					},
				},
				// Condition: "file.permissions == 0644",
				Type: "object",
			},
			module: fmt.Sprintf(objectModule, "true"),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				_, filePaths := createTempFiles(t, 1)

				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("NormalizeToHostRoot", file.Path).Return(filePaths[0])
				env.On("RelativeToHostRoot", filePaths[0]).Return(file.Path)
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Equal(file.Path, report.Data["file.path"])
			},
			maxPermissions: "644",
		},
		{
			name: "file permissions (glob)",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Glob: "/etc/*.dat",
					},
				},
				// Condition: "file.permissions == 0644",
			},
			module: fmt.Sprintf(arrayModule, "true"),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()

				tempDir, filePaths := createTempFiles(t, 2)
				for _, filePath := range filePaths {
					env.On("RelativeToHostRoot", filePath).Return(path.Join("/etc/", path.Base(filePath)))
				}

				env.On("NormalizeToHostRoot", file.Glob).Return(path.Join(tempDir, "/*.dat"))
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Regexp("/etc/test-[0-9]-[0-9]+", report.Data["file.path"])
			},
			maxPermissions: "644",
		},
		{
			name: "file user and group",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path: "/tmp",
					},
				},
				Type: "object",
				// Condition: `file.user == "root" && file.group in ["root", "wheel"]`,
			},
			module: fmt.Sprintf(objectModule, `true`),
			setup:  normalizePath,
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Equal("/tmp", report.Data["file.path"])
				assert.Equal("root", report.Data["file.user"])
				assert.Contains([]string{"root", "wheel"}, report.Data["file.group"])
			},
		},
		{
			name: "jq(log-driver) - passed",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   "/etc/docker/daemon.json",
						Parser: "json",
					},
				},
				Type: "object",
				// Condition: `file.jq(".\"log-driver\"") == "json-file"`,
			},
			module: fmt.Sprintf(objectModule, `file.content["log-driver"] == "json-file"`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/daemon.json").Return(file.Path)
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Equal("/etc/docker/daemon.json", report.Data["file.path"])
				assert.NotEmpty(report.Data["file.user"])
				assert.NotEmpty(report.Data["file.group"])
			},
		},
		{
			name: "jq(experimental) - failed",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   "/etc/docker/daemon.json",
						Parser: "json",
					},
				},
				Type: "object",
				// Condition: `file.jq(".experimental") == "true"`,
			},
			module: fmt.Sprintf(objectModule, `file.content["experimental"] == true`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/daemon.json").Return(file.Path)
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.False(report.Passed)
				assert.Equal("/etc/docker/daemon.json", report.Data["file.path"])
				assert.NotEmpty(report.Data["file.user"])
				assert.NotEmpty(report.Data["file.group"])
			},
		},
		{
			name: "jq(experimental) and path expression",
			additionalResources: []compliance.RegoInput{{
				ResourceCommon: compliance.ResourceCommon{
					Process: &compliance.Process{
						Name: "dockerd",
					},
				},
				Type: "object",
				// Condition: `file.jq(".experimental") == "false"`,
			}},
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   `process.flag("dockerd", "--config-file")`,
						Parser: "json",
					},
				},
				Type: "object",
				// Condition: `file.jq(".experimental") == "false"`,
				Transform: `h.file_process_flag("--config-file")`,
			},
			processes: processutils.Processes{
				42: processutils.NewCheckedFakeProcess(42, "dockerd", []string{"dockerd", "--config-file=/etc/docker/daemon.json"}),
			},
			module: fmt.Sprintf(objectModule, `file.content["experimental"] == false`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				path := "/etc/docker/daemon.json"
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("NormalizeToHostRoot", path).Return("./testdata/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/daemon.json").Return(path)
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Equal("/etc/docker/daemon.json", report.Data["file.path"])
				assert.NotEmpty(report.Data["file.user"])
				assert.NotEmpty(report.Data["file.group"])
			},
		},
		{
			name: "jq(experimental) and path expression",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   `process.flag("dockerd", "--config-file")`,
						Parser: "json",
					},
				},
				Type: "object",
				// Condition: `file.jq(".experimental") == "false"`,
			},
			module: fmt.Sprintf(objectModule, `file.content["experimental"] == false`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				path := "/etc/docker/daemon.json"
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("EvaluateFromCache", mock.Anything).Return(path, nil)
				env.On("NormalizeToHostRoot", path).Return("./testdata/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/daemon.json").Return(path)
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Equal("/etc/docker/daemon.json", report.Data["file.path"])
				assert.NotEmpty(report.Data["file.user"])
				assert.NotEmpty(report.Data["file.group"])
			},
		},
		{
			name: "jq(experimental) and path expression - empty path",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   `process.flag("dockerd", "--config-file")`,
						Parser: "json",
					},
				},
				Type: "object",
				// Condition: `file.jq(".experimental") == "false"`,
			},
			module: fmt.Sprintf(objectModule, `file.content["experimental"] == false`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("EvaluateFromCache", mock.Anything).Return("", nil)
				setDefaultHooks(env)
			},
			expectError: errors.New(`failed to resolve path: empty path from process.flag("dockerd", "--config-file")`),
		},
		{
			name: "jq(experimental) and path expression - wrong type",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   `process.flag("dockerd", "--config-file")`,
						Parser: "json",
					},
				},
				Type: "object",
				// Condition: `file.jq(".experimental") == "false"`,
			},
			module: fmt.Sprintf(objectModule, `file.content["experimental"] == false`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("EvaluateFromCache", mock.Anything).Return(true, nil)
				setDefaultHooks(env)
			},
			expectError: errors.New(`failed to resolve path`),
			processes: processutils.Processes{
				42: processutils.NewCheckedFakeProcess(42, "dockerd", []string{"dockerd", "--config-file=/etc/docker/daemon.json"}),
			},
		},
		{
			name: "jq(experimental) and path expression - expression failed",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   `process.unknown()`,
						Parser: "json",
					},
				},
				Type: "object",
				// Condition: `file.jq(".experimental") == "false"`,
			},
			module: fmt.Sprintf(objectModule, `file.content["experimental"] == false`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("EvaluateFromCache", mock.Anything).Return(nil, errors.New("1:1: unknown function process.unknown()"))
				setDefaultHooks(env)
			},
			expectError: errors.New(`failed to resolve path`),
		},
		{
			name: "jq(ulimits)",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   "/etc/docker/daemon.json",
						Parser: "json",
					},
				},
				Type: "object",
				// Condition: `file.jq(".[\"default-ulimits\"].nofile.Hard") == "64000"`,
			},
			module: fmt.Sprintf(objectModule, `file.content["default-ulimits"].nofile.Hard == 64000`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/daemon.json")
				env.On("RelativeToHostRoot", "./testdata/daemon.json").Return(file.Path)
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Equal("/etc/docker/daemon.json", report.Data["file.path"])
				assert.NotEmpty(report.Data["file.user"])
				assert.NotEmpty(report.Data["file.group"])
			},
		},
		{
			name: "yaml(apiVersion)",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   "/etc/pod.yaml",
						Parser: "yaml",
					},
				},
				Type: "object",
				// Condition: `file.yaml(".apiVersion") == "v1"`,
			},
			module: fmt.Sprintf(objectModule, `file.content["apiVersion"] == "v1"`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/pod.yaml")
				env.On("RelativeToHostRoot", "./testdata/pod.yaml").Return(file.Path)
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Equal("/etc/pod.yaml", report.Data["file.path"])
				assert.NotEmpty(report.Data["file.user"])
				assert.NotEmpty(report.Data["file.group"])
			},
		},
		{
			name: "regexp",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path:   "/proc/mounts",
						Parser: "raw",
					},
				},
				Type: "object",
				// Condition: `file.regexp("[a-zA-Z0-9-_/]+ /boot/efi [a-zA-Z0-9-_/]+") != ""`,
			},
			module: fmt.Sprintf(objectModule, `regex.match("[a-zA-Z0-9-_/]+ /boot/efi [a-zA-Z0-9-_/]+", file.content)`),
			setup: func(t *testing.T, env *mocks.Env, file *compliance.File) {
				env.On("MaxEventsPerRun").Return(30).Maybe()
				env.On("NormalizeToHostRoot", file.Path).Return("./testdata/mounts")
				env.On("RelativeToHostRoot", "./testdata/mounts").Return(file.Path)
				setDefaultHooks(env)
			},
			validate: func(t *testing.T, file *compliance.File, report *compliance.Report) {
				assert.True(report.Passed)
				assert.Equal("/proc/mounts", report.Data["file.path"])
				assert.NotEmpty(report.Data["file.user"])
				assert.NotEmpty(report.Data["file.group"])
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

			if len(test.processes) > 0 {
				previousFetcher := processutils.Fetcher
				processutils.Fetcher = func() (processutils.Processes, error) {
					return test.processes, nil
				}
				defer func() {
					processutils.Fetcher = previousFetcher
				}()
			}

			regoRule := resource_test.NewTestRule(test.resource, "file", test.module)
			if test.maxPermissions != "" {
				regoRule.Inputs[1].Constants.Values["max_permissions"] = test.maxPermissions
			} else {
				regoRule.Inputs[1].Constants.Values["max_permissions"] = "777"
			}
			regoRule.Inputs[1].Constants.Values["resource_type"] = "docker_daemon"
			regoRule.Inputs = append(test.additionalResources, regoRule.Inputs...)
			fileCheck := rego.NewCheck(regoRule)
			err := fileCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
			assert.NoError(err)

			reports := fileCheck.Check(env)
			jsonReports, err := json.MarshalIndent(reports, "", "  ")
			assert.NoError(err)
			t.Log(string(jsonReports))

			if test.expectError != nil {
				assert.Contains(reports[0].Error.Error(), test.expectError.Error())
			} else {
				assert.NoError(err)
				test.validate(t, test.resource.File, reports[0])
			}
		})
	}
}
