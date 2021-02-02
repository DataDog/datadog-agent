// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !windows

package flare

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestCreateSecurityAgentArchive(t *testing.T) {
	assert := assert.New(t)

	common.SetupConfig("./test")
	mockConfig := config.Mock()
	mockConfig.Set("compliance_config.dir", "./test/compliance.d")
	logFilePath := "./test/logs/agent.log"

	tests := []struct {
		name          string
		local         bool
		expectedFiles []string
	}{
		{
			name:  "local flare",
			local: true,
			expectedFiles: []string{
				"compliance.d/cis-docker.yaml",
				"logs/agent.log",
			},
		},
		{
			name:  "non local flare",
			local: false,
			expectedFiles: []string{
				"compliance.d/cis-docker.yaml",
				"logs/agent.log",
				"security-agent-status.log",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			zipFilePath, err := CreateSecurityAgentArchive(test.local, logFilePath, nil)
			defer os.Remove(zipFilePath)

			assert.NoError(err)

			// asserts that it as indeed created a permissions.log file
			z, err := zip.OpenReader(zipFilePath)
			assert.NoError(err, "opening the zip shouldn't pop an error")

			var fileNames []string
			for _, f := range z.File {
				fileNames = append(fileNames, f.Name)
			}

			dir := fileNames[0]
			for _, f := range test.expectedFiles {
				assert.Contains(fileNames, filepath.Join(dir, f))
			}
		})
	}
}
