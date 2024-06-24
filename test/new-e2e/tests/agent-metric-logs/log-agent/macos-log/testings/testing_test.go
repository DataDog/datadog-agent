// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package macostestings

import (
	_ "embed"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	testos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

// MacOSFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type MacOSFakeintakeSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestE2EVMFakeintakeSuite runs the E2E test suite for the log agent with a VM and fake intake.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithEC2InstanceOptions(ec2.WithOS(testos.UbuntuDefault)))),
	}
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}

	e2e.Run(t, &MacOSFakeintakeSuite{}, options...)
}

func (s *MacOSFakeintakeSuite) TestOS() {
	s.Run("Setup", s.testSetup)
}

func (s *MacOSFakeintakeSuite) testSetup() {
	t := s.T()
	ls, err := s.Env().RemoteHost.Execute("ls")
	assert.NoErrorf(t, err, "Failed to list files in the directory")
	assert.NotEmptyf(t, ls, "ls is not giving any output")
}
