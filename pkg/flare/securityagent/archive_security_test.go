// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package securityagent

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/fx"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	// Required to initialize the "dogstatsd" expvar
	_ "github.com/DataDog/datadog-agent/comp/dogstatsd/server/impl"
	_ "github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
)

func TestCreateSecurityAgentArchive(t *testing.T) {
	mockConfig := configmock.New(t)
	statusComponent := fxutil.Test[status.Mock](t, fx.Options(
		statusimpl.MockModule(),
	))

	complianceDir, logFilePath := setupSecurityAgentTestdata(t)
	mockConfig.SetInTest("compliance_config.dir", complianceDir)

	// Mock getLinuxKernelSymbols. It can take a long time to scrub when creating a flare.
	defer func(f func(flaretypes.FlareBuilder) error) {
		linuxKernelSymbols = f
	}(getLinuxKernelSymbols)
	linuxKernelSymbols = func(fb flaretypes.FlareBuilder) error {
		fb.AddFile("kallsyms", []byte("some kernel symbol"))
		return nil
	}

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
			createSecurityAgentArchive(mock, logFilePath, statusComponent)

			for _, f := range test.expectedFiles {
				mock.AssertFileExists(f)
			}
		})
	}
}

func setupSecurityAgentTestdata(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	complianceDir := filepath.Join(root, "compliance.d")
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(complianceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(complianceDir, "cis-docker.yaml"), []byte("name: CIS Docker Generic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logFilePath := filepath.Join(logDir, "agent.log")
	if err := os.WriteFile(logFilePath, []byte("agent log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return complianceDir, logFilePath
}
