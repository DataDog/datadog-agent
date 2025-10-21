package os

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/command"
)

func newMacOS(e config.Env, desc Descriptor, runner command.Runner) OS {
	os := &os{
		descriptor:     desc,
		runner:         runner,
		fileManager:    command.NewFileManager(runner),
		packageManager: newBrewManager(runner),
		serviceManager: newMacOSServiceManager(e, runner),
	}

	return os
}
