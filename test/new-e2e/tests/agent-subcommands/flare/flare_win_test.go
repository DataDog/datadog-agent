// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package flare

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"

	"github.com/stretchr/testify/assert"
)

type windowsFlareSuite struct {
	baseFlareSuite
}

func TestWindowsFlareSuite(t *testing.T) {
	e2e.Run(t, &windowsFlareSuite{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

// Add zz to name to run this test last in order to don't break other tests
// Will need to rename it to TestFlareWithAllConfiguration after the fix of Paola's PR
// To keep in mind that we will need to create the directory then create file in it and specify to delete file as the same time as the directory
func (v *windowsFlareSuite) TestzzzFlareWindows() {
	flare, _ := requestAgentFlareAndFetchFromFakeIntake(&v.baseFlareSuite, agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

	assertFilesExist(v.T(), flare, windowsFiles)
	assertEventlogFolderOnlyContainsWindoesEventLog(v.T(), flare)

	expectedCounterStrings := []string{"Write Packets/sec", "Events Logged per sec"}
	assertFileContains(v.T(), flare, "counter_strings.txt", expectedCounterStrings...)

	_, err := flare.GetFile("datadog-raw.reg")
	assert.Error(v.T(), err, "File 'datadog-raw.reg' was found in flare, but was expected not to be part of the archive")
}
