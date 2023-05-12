// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package flare

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/config"

	// Required to initialize the "dogstatsd" expvar
	_ "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

func TestCreateSecurityAgentArchive(t *testing.T) {
	common.SetupConfig("./test")
	mockConfig := config.Mock(t)
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
			mock := flarehelpers.NewFlareBuilderMock(t, test.local)
			createSecurityAgentArchive(mock.Fb, logFilePath, nil, nil)

			for _, f := range test.expectedFiles {
				mock.AssertFileExists(f)
			}
		})
	}
}
