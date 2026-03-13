// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type systemdServiceManager struct {
	e      config.Env
	runner command.Runner
}

func newSystemdServiceManager(e config.Env, runner command.Runner) ServiceManager {
	return &systemdServiceManager{e: e, runner: runner}
}

func (s *systemdServiceManager) EnsureRestarted(serviceName string, transform command.Transformer, opts ...pulumi.ResourceOption) (command.Command, error) {
	cmdName := s.e.CommonNamer().ResourceName("running", serviceName)
	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Sudo:   true,
		Create: pulumi.String("systemctl restart " + serviceName),
	}

	// If a transform is provided, use it to modify the command name and args
	if transform != nil {
		cmdName, cmdArgs = transform(cmdName, cmdArgs)
	}

	return s.runner.Command(cmdName, cmdArgs, opts...)
}

type sysvinitServiceManager struct {
	e      config.Env
	runner command.Runner
}

func newSysvinitServiceManager(e config.Env, runner command.Runner) ServiceManager {
	return &sysvinitServiceManager{e: e, runner: runner}
}

func (s *sysvinitServiceManager) EnsureRestarted(serviceName string, transform command.Transformer, opts ...pulumi.ResourceOption) (command.Command, error) {
	cmdName := s.e.CommonNamer().ResourceName("running", serviceName)
	// To the difference of systemctl the restart doesn't work if the service isn't already running
	// so instead we run a stop command that we allow to fail and then a start command
	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Sudo:   false,
		Create: pulumi.String(fmt.Sprintf("sudo stop %[1]s; sudo start %[1]s", serviceName)),
	}

	// If a transform is provided, use it to modify the command name and args
	if transform != nil {
		cmdName, cmdArgs = transform(cmdName, cmdArgs)
	}

	return s.runner.Command(cmdName, cmdArgs, opts...)
}
