// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"fmt"
	"strings"

	"github.com/DataDog/test-infra-definitions/common/config"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
)

// WindowsHost is an environment based on environments.Host but that is specific to Windows.
type WindowsHost struct {
	Environment config.Env
	// Components
	RemoteHost      *components.RemoteHost
	FakeIntake      *components.FakeIntake
	Agent           *components.RemoteHostAgent
	ActiveDirectory *components.RemoteActiveDirectory
}

var _ common.Initializable = &WindowsHost{}

// Init initializes the environment
func (e *WindowsHost) Init(_ common.Context) error {
	return nil
}

// Diagnose returns a string containing the diagnosis of the environment
func (e *WindowsHost) Diagnose(outputDir string) (string, error) {
	diagnoses := []string{}
	if e.RemoteHost == nil {
		return "", fmt.Errorf("RemoteHost component is not initialized")
	}
	// add Agent diagnose
	if e.Agent != nil {
		diagnoses = append(diagnoses, "==== Agent ====")
		dstPath, err := generateAndDownloadAgentFlare(e.Agent, e.RemoteHost, outputDir)
		if err != nil {
			return "", fmt.Errorf("failed to generate and download agent flare: %w", err)
		}
		diagnoses = append(diagnoses, fmt.Sprintf("Flare archive downloaded to %s", dstPath))
		diagnoses = append(diagnoses, "\n")
	}

	return strings.Join(diagnoses, "\n"), nil
}
