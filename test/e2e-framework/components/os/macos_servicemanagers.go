package os

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/command"
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
