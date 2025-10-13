package os

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func newZypperManager(runner command.Runner) *ZypperPackageManager {
	return &ZypperPackageManager{
		namer:      namer.NewNamer(runner.Environment().Ctx(), "zypper"),
		runner:     runner,
		pulumiOpts: []pulumi.ResourceOption{},
	}
}

type ZypperPackageManager struct {
	namer      namer.Namer
	runner     command.Runner
	pulumiOpts []pulumi.ResourceOption
}

func (m *ZypperPackageManager) Ensure(packageRef string, transform command.Transformer, checkBinary string, opts ...PackageManagerOption) (command.Command, error) {
	params, err := common.ApplyOption(&PackageManagerParams{}, opts)
	if err != nil {
		return nil, err
	}

	pulumiOpts := append(params.PulumiResourceOptions, m.pulumiOpts...)

	zypperInstallCmd := "zypper -n install"
	if params.AllowUnsignedPackages {
		zypperInstallCmd = "zypper -n --no-gpg-checks install"
	}

	var cmdStr string
	if checkBinary != "" {
		cmdStr = fmt.Sprintf("bash -c 'command -v %s || %s %s'", checkBinary, zypperInstallCmd, packageRef)
	} else {
		cmdStr = fmt.Sprintf("%s %s", zypperInstallCmd, packageRef)
	}

	cmdName := m.namer.ResourceName("install-"+packageRef, utils.StrHash(cmdStr))
	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Create: pulumi.String(cmdStr),
		Sudo:   true,
	}

	// If a transform is provided, use it to modify the command name and args
	if transform != nil {
		cmdName, cmdArgs = transform(cmdName, cmdArgs)
	}

	cmd, err := m.runner.Command(cmdName, cmdArgs, pulumiOpts...)
	if err != nil {
		return nil, err
	}

	// Make sure the package manager isn't running in parallel
	m.pulumiOpts = append(m.pulumiOpts, utils.PulumiDependsOn(cmd))
	return cmd, nil
}

func (m *ZypperPackageManager) EnsureUninstalled(packageRef string, transform command.Transformer, checkBinary string, opts ...PackageManagerOption) (command.Command, error) {
	params, err := common.ApplyOption(&PackageManagerParams{}, opts)
	if err != nil {
		return nil, err
	}

	pulumiOpts := append(params.PulumiResourceOptions, m.pulumiOpts...)
	// Ensure the package is uninstalled
	zypperUninstallCmd := "zypper -n remove"

	var cmdStr string
	if checkBinary != "" {
		cmdStr = fmt.Sprintf("bash -c 'command -v %s && %s %s || exit 0'", checkBinary, zypperUninstallCmd, packageRef)
	} else {
		cmdStr = fmt.Sprintf("%s %s", zypperUninstallCmd, packageRef)
	}

	cmdName := m.namer.ResourceName("uninstall-"+packageRef, utils.StrHash(cmdStr))
	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Create: pulumi.String(cmdStr),
		Sudo:   true,
	}

	// If a transform is provided, use it to modify the command name and args
	if transform != nil {
		cmdName, cmdArgs = transform(cmdName, cmdArgs)
	}

	cmd, err := m.runner.Command(cmdName, cmdArgs, pulumiOpts...)
	if err != nil {
		return nil, err
	}

	// Make sure the package manager isn't running in parallel
	m.pulumiOpts = append(m.pulumiOpts, utils.PulumiDependsOn(cmd))
	return cmd, nil
}
