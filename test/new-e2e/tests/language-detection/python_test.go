// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/stretchr/testify/require"

	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

func (s *languageDetectionSuite) installPython() {
	s.Env().RemoteHost.MustExecute("sudo apt-get -y install python3")
	pyVersion := s.Env().RemoteHost.MustExecute("python3 --version")
	require.True(s.T(), strings.HasPrefix(pyVersion, "Python 3"))
}

func (s *languageDetectionSuite) TestPythonDetectionCoreAgent() {
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(getProvisionerOptions([]func(*agentparams.Params) error{
		agentparams.WithAgentConfig(coreConfigStr),
	})...))
	pid := s.startPython()
	s.checkDetectedLanguage(pid, "python", "process_collector")
}

func (s *languageDetectionSuite) TestPythonDetectionCoreAgentNoCheck() {
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(getProvisionerOptions([]func(*agentparams.Params) error{
		agentparams.WithAgentConfig(coreConfigNoCheckStr),
	})...))
	pid := s.startPython()
	s.checkDetectedLanguage(pid, "python", "process_collector")
}

func (s *languageDetectionSuite) TestPythonDetectionProcessAgent() {
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(getProvisionerOptions([]func(*agentparams.Params) error{
		agentparams.WithAgentConfig(processConfigStr),
	})...))
	pid := s.startPython()
	s.checkDetectedLanguage(pid, "python", "process_collector")
}

func (s *languageDetectionSuite) TestPythonDetectionProcessAgentNoCheck() {
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(getProvisionerOptions([]func(*agentparams.Params) error{
		agentparams.WithAgentConfig(processConfigNoCheckStr),
	})...))
	pid := s.startPython()
	s.checkDetectedLanguage(pid, "python", "process_collector")
}

func (s *languageDetectionSuite) startPython() string {
	s.Env().RemoteHost.MustExecute("echo -e 'import time\ntime.sleep(60)' > prog.py")
	return s.Env().RemoteHost.MustExecute("nohup python3 prog.py >myscript.log 2>&1 </dev/null & echo -n $!")
}
