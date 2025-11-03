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

type macOSServiceManager struct {
	e      config.Env
	runner command.Runner
}

func newMacOSServiceManager(e config.Env, runner command.Runner) ServiceManager {
	return &macOSServiceManager{e: e, runner: runner}
}

func (s *macOSServiceManager) EnsureRestarted(serviceName string, transform command.Transformer, opts ...pulumi.ResourceOption) (command.Command, error) {
	cmdName := s.e.CommonNamer().ResourceName("running", serviceName)
	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Sudo:   true,
		Create: pulumi.String(fmt.Sprintf("launchctl kickstart -k %s'", serviceName)),
	}

	// If a transform is provided, use it to modify the command name and args
	if transform != nil {
		cmdName, cmdArgs = transform(cmdName, cmdArgs)
	}

	return s.runner.Command(cmdName, cmdArgs, opts...)
}
