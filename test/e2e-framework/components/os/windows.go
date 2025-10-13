package os

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/command"
)

func newWindowsOS(e config.Env, desc Descriptor, runner command.Runner) OS {
	os := &os{
		descriptor:     desc,
		runner:         runner,
		fileManager:    command.NewFileManager(runner),
		serviceManager: newWindowsServiceManager(e, runner),
	}

	return os
}
