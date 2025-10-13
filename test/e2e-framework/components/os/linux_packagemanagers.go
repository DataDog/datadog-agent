package os

import (
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func newAptManager(runner command.Runner) PackageManager {
	return NewGenericPackageManager(runner, "apt", "apt-get install -y", "apt-get update -y", "apt-get remove -y",
		pulumi.StringMap{"DEBIAN_FRONTEND": pulumi.String("noninteractive")})
}

func newYumManager(runner command.Runner) PackageManager {
	return NewGenericPackageManager(runner, "yum", "yum install -y", "", "yum remove -y", nil)
}

func newDnfManager(runner command.Runner) PackageManager {
	return NewGenericPackageManager(runner, "dnf", "dnf install -y", "", "dnf remove -y", nil)
}
