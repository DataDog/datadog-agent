// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for the rtloader
package rtloader

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	osVM "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

//go:embed python-check/multi_file_check.py
var MultiFileCheckPy []byte

type baseMultiProcessingLibSuite struct {
	e2e.BaseSuite[environments.Host]

	confdPath   string
	checksdPath string

	tempDir string
}

func (v *baseMultiProcessingLibSuite) getSuiteOptions(osInstance osVM.Descriptor) []e2e.SuiteOption {
	suiteOptions := []e2e.SuiteOption{
		e2e.WithDevMode(),
	}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				agentparams.WithFile(v.confdPath, `
init_config:

instances:
  - name: "default"
`, true),
				agentparams.WithFile(v.checksdPath, string(MultiFileCheckPy), true),
			),
			awshost.WithEC2InstanceOptions(ec2.WithOS(osInstance)),
		),
	))

	return suiteOptions
}

func (v *baseMultiProcessingLibSuite) TestMultiProcessingLib() {
	v.T().Log("Running MultiProcessingLib test")

	// Wait for the check to run and verify its status
	time.Sleep(10 * time.Second) // Give time for the check to run
	statusOutput := v.Env().Agent.Client.Status()
	require.Contains(v.T(), statusOutput.Content, "multi_file_check")

	// Check the output files for different PIDs
	filePaths := []string{
		filepath.Join(v.tempDir, "file1.txt"),
		filepath.Join(v.tempDir, "file2.txt"),
		filepath.Join(v.tempDir, "file3.txt"),
	}

	// Read first line of each file and verify different PIDs
	pids := make(map[string]bool)
	for _, filePath := range filePaths {
		var content string
		if v.Env().RemoteHost.OSFamily == osVM.WindowsDefault.Family() {
			content = v.Env().RemoteHost.MustExecute(fmt.Sprintf("Get-Content -Path '%s' -TotalCount 1", filePath))
		} else {
			content = v.Env().RemoteHost.MustExecute(fmt.Sprintf("head -n 1 %s", filePath))
		}
		// Extract PID from the line
		pid := strings.Split(content, "Process ")[1]
		pid = strings.Split(pid, "]")[0]
		pids[pid] = true
	}

	// Verify we have at least 2 different PIDs
	assert.GreaterOrEqual(v.T(), len(pids), 2, "Expected at least 2 different PIDs in the output files")
}
