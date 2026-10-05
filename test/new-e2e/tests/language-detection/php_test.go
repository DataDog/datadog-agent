// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"

	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

func (s *languageDetectionSuite) TestPHPDetectionCoreAgent() {
	s.UpdateEnv(awshost.ProvisionerNoFakeIntake(getProvisionerOptions([]func(*agentparams.Params) error{
		agentparams.WithAgentConfig(coreConfigStr),
	})...))
	pid := s.startPHP()
	s.checkDetectedLanguage(pid, "php", "process_collector")
}

func (s *languageDetectionSuite) startPHP() string {
	s.Env().RemoteHost.MustExecute("echo -e '<?php sleep(60);' > prog.php")
	return s.Env().RemoteHost.MustExecute("nohup php prog.php >myscript.log 2>&1 </dev/null & echo -n $!")
}
