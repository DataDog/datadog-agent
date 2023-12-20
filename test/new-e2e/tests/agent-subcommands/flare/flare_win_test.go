// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package flare

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"
)

type windowsFlareSuite struct {
	baseFlareSuite
}

func TestWindowsFlareSuite(t *testing.T) {
	e2e.Run(t, &windowsFlareSuite{}, e2e.FakeIntakeStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS))))
}

func (v *windowsFlareSuite) TestFlareWindows() {
	v.UpdateEnv(e2e.FakeIntakeStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS))))
	flare := requestAgentFlareAndFetchFromFakeIntake(v.T(), v.Env().Agent, v.Env().Fakeintake, client.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

	assertFilesExist(v.T(), flare, windowsFiles)
	assertEventlogFolderOnlyContainsWindoesEventLog(v.T(), flare)

	expectedCounterStrings := []string{"Write Packets/sec", "Events Logged per sec"}
	assertFileContains(v.T(), flare, "counter_strings.txt", expectedCounterStrings...)

	_, err := flare.GetFile("datadog-raw.reg")
	assert.Error(v.T(), err, "File 'datadog-raw.reg' was found in flare, but was expected not to be part of the archive")
}
