package os

import (
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func newBrewManager(runner command.Runner) PackageManager {
	return NewGenericPackageManager(runner, "brew", "brew install -y", "brew update -y", "brew uninstall -y",
		pulumi.StringMap{"NONINTERACTIVE": pulumi.String("1")})
}
