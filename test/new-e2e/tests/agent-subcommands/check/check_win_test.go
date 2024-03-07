// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check contains helpers and e2e tests of the check command
package check

// TODO: not working yet because of the following error:
// unable to import module 'hello': source code string cannot contain null bytes
// Uncomment the code below when the issue is fixed
/*type windowsCheckSuite struct {
	baseCheckSuite
}

func TestWindowsCheckSuite(t *testing.T) {
	e2e.Run(t, &windowsCheckSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
			awshost.WithAgentOptions(
				agentparams.WithFile("C:/ProgramData/Datadog/conf.d/hello.d/conf.yaml", string(customCheckYaml), true),
				agentparams.WithFile("C:/ProgramData/Datadog/checks.d/hello.py", string(customCheckPython), true),
			))), e2e.WithDevMode())
}*/
