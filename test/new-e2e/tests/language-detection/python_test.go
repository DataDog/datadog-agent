// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"strings"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/require"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

func (s *languageDetectionSuite) installPython() {
	s.Env().RemoteHost.MustExecute("sudo apt-get -y install python3")
	pyVersion := s.Env().RemoteHost.MustExecute("python3 --version")
	require.True(s.T(), strings.HasPrefix(pyVersion, "Python 3"))
}

func (s *languageDetectionSuite) TestPythonDetectionCoreAgent() {
	s.T().Skip("Skipping test as this feature is not currently usable")
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig(coreConfigStr))))
	s.runPython()
	s.checkDetectedLanguage("python3", "python", "local_process_collector")
}

func (s *languageDetectionSuite) TestPythonDetectionCoreAgentNoCheck() {
	s.T().Skip("Skipping test as this feature is not currently usable")
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig(coreConfigNoCheckStr))))
	s.runPython()
	s.checkDetectedLanguage("python3", "python", "local_process_collector")
}

func (s *languageDetectionSuite) TestPythonDetectionProcessAgent() {
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig(processConfigStr))))
	s.runPython()
	s.checkDetectedLanguage("python3", "python", "remote_process_collector")
}

func (s *languageDetectionSuite) TestPythonDetectionProcessAgentNoCheck() {
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig(processConfigNoCheckStr))))
	s.runPython()
	s.checkDetectedLanguage("python3", "python", "remote_process_collector")
}

func (s *languageDetectionSuite) runPython() {
	s.Env().RemoteHost.MustExecute("echo -e 'import time\ntime.sleep(60)' > prog.py")
	s.Env().RemoteHost.MustExecute("nohup python3 prog.py >myscript.log 2>&1 </dev/null &")
}
