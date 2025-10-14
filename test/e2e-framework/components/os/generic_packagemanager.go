package os

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type GenericPackageManager struct {
	namer           namer.Namer
	updateDBCommand command.Command
	runner          command.Runner
	opts            []pulumi.ResourceOption
	installCmd      string
	updateCmd       string
	uninstallCmd    string
	env             pulumi.StringMap
}

func NewGenericPackageManager(
	runner command.Runner,
	name string,
	installCmd string,
	updateCmd string,
	uninstallCmd string,
	env pulumi.StringMap,
) *GenericPackageManager {
	packageManager := &GenericPackageManager{
		namer:        namer.NewNamer(runner.Environment().Ctx(), name),
		runner:       runner,
		installCmd:   installCmd,
		updateCmd:    updateCmd,
		uninstallCmd: uninstallCmd,
		env:          env,
	}

	return packageManager
}

func (m *GenericPackageManager) EnsureUninstalled(packageRef string, transform command.Transformer, checkBinary string, opts ...PackageManagerOption) (command.Command, error) {
	params, err := common.ApplyOption(&PackageManagerParams{}, opts)
	if err != nil {
		return nil, err
	}
	pulumiOpts := append(params.PulumiResourceOptions, m.opts...)
	var cmdStr string
	if checkBinary != "" {
		cmdStr = fmt.Sprintf("bash -c 'command -v %s && %s %s || exit 0'", checkBinary, m.uninstallCmd, packageRef)
	} else if m.uninstallCmd != "" {
		cmdStr = fmt.Sprintf("%s %s", m.uninstallCmd, packageRef)
	} else {
		return nil, fmt.Errorf("uninstall command is not set")
	}

	cmdName := m.namer.ResourceName("uninstall-"+packageRef, utils.StrHash(cmdStr))
	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Create:      pulumi.String(cmdStr),
		Environment: m.env,
		Sudo:        true,
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
	m.opts = append(m.opts, utils.PulumiDependsOn(cmd))

	return cmd, nil
}
func (m *GenericPackageManager) Ensure(packageRef string, transform command.Transformer, checkBinary string, opts ...PackageManagerOption) (command.Command, error) {
	params, err := common.ApplyOption(&PackageManagerParams{}, opts)
	if err != nil {
		return nil, err
	}

	pulumiOpts := append(params.PulumiResourceOptions, m.opts...)
	if m.updateCmd != "" {
		updateDB, err := m.updateDB(pulumiOpts)
		if err != nil {
			return nil, err
		}

		pulumiOpts = append(pulumiOpts, utils.PulumiDependsOn(updateDB))
	}
	var cmdStr string
	if checkBinary != "" {
		cmdStr = fmt.Sprintf("bash -c 'command -v %s || %s %s'", checkBinary, m.installCmd, packageRef)
	} else {
		cmdStr = fmt.Sprintf("%s %s", m.installCmd, packageRef)
	}

	cmdName := m.namer.ResourceName("install-"+packageRef, utils.StrHash(cmdStr))
	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Create:      pulumi.String(cmdStr),
		Environment: m.env,
		Sudo:        true,
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
	m.opts = append(m.opts, utils.PulumiDependsOn(cmd))
	return cmd, nil
}

func (m *GenericPackageManager) updateDB(opts []pulumi.ResourceOption) (command.Command, error) {
	if m.updateDBCommand != nil {
		return m.updateDBCommand, nil
	}

	c, err := m.runner.Command(
		m.namer.ResourceName("update"),
		&command.Args{
			Create:      pulumi.String(m.updateCmd),
			Environment: m.env,
			Sudo:        true,
		}, opts...)
	if err == nil {
		m.updateDBCommand = c
	}

	return c, err
}
