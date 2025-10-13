package os

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/command"
)

func newLinuxOS(e config.Env, desc Descriptor, runner command.Runner) OS {
	os := &os{
		descriptor:  desc,
		runner:      runner,
		fileManager: command.NewFileManager(runner),
	}

	switch desc.Flavor {
	case AmazonLinux, AmazonLinuxECS, CentOS:
		// AL2 is YUM, AL2023 is DNF (but with yum compatibility)
		os.packageManager = newYumManager(runner)

	case Fedora, RedHat, RockyLinux:
		os.packageManager = newDnfManager(runner)

	case Debian, Ubuntu:
		os.packageManager = newAptManager(runner)

	case Suse:
		os.packageManager = newZypperManager(runner)

	case Unknown, WindowsServer, MacosOS:
		fallthrough
	default:
		panic(fmt.Sprintf("unsupported linux flavor from desc: %+v", desc))
	}

	if desc.Flavor == AmazonLinux2018.Flavor && desc.Version == AmazonLinux2018.Version {
		os.serviceManager = newSysvinitServiceManager(e, runner)
	} else {
		os.serviceManager = newSystemdServiceManager(e, runner)
	}

	return os
}
