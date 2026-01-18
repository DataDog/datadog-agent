// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"fmt"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
)

type RunnerCommandArgs interface {
	Arguments() *Args
}

type Args struct {
	Create                   pulumi.StringInput
	Update                   pulumi.StringInput
	Delete                   pulumi.StringInput
	Triggers                 pulumi.ArrayInput
	Stdin                    pulumi.StringPtrInput
	Environment              pulumi.StringMap
	RequirePasswordFromStdin bool
	Sudo                     bool
	// Only used for local commands
	LocalAssetPaths pulumi.StringArrayInput
	LocalDir        pulumi.StringInput
}

type LocalArgs struct {
	Args
	// Only used for local commands
	LocalAssetPaths pulumi.StringArrayInput
	LocalDir        pulumi.StringInput
}

var _ RunnerCommandArgs = &Args{}
var _ RunnerCommandArgs = &LocalArgs{}

func (args *Args) Arguments() *Args {
	return args
}

func (args *LocalArgs) Arguments() *Args {
	return &args.Args
}

func toLocalCommandArgs(cmdArgs RunnerCommandArgs, config RunnerConfiguration, osCommand OSCommand) (*local.CommandArgs, error) {
	// Retrieve local specific arguments if provided
	var assetsPath pulumi.StringArrayInput
	var dir pulumi.StringInput
	if localArgs, ok := cmdArgs.(*LocalArgs); ok {
		assetsPath = localArgs.LocalAssetPaths
		dir = localArgs.LocalDir
	}

	args := cmdArgs.Arguments()

	return &local.CommandArgs{
		Create:     osCommand.BuildCommandString(args.Create, args.Environment, args.Sudo, args.RequirePasswordFromStdin, config.user),
		Update:     osCommand.BuildCommandString(args.Update, args.Environment, args.Sudo, args.RequirePasswordFromStdin, config.user),
		Delete:     osCommand.BuildCommandString(args.Delete, args.Environment, args.Sudo, args.RequirePasswordFromStdin, config.user),
		Triggers:   args.Triggers,
		Stdin:      args.Stdin,
		AssetPaths: assetsPath,
		Dir:        dir,
	}, nil
}

func toRemoteCommandArgs(cmdArgs RunnerCommandArgs, config RunnerConfiguration, osCommand OSCommand) (*remote.CommandArgs, error) {
	// Ensure no local arguments are passed to remote commands
	if _, ok := cmdArgs.(*LocalArgs); ok {
		return nil, fmt.Errorf("local arguments are not allowed for remote commands")
	}

	args := cmdArgs.Arguments()

	return &remote.CommandArgs{
		Connection: config.connection,
		Create:     osCommand.BuildCommandString(args.Create, args.Environment, args.Sudo, args.RequirePasswordFromStdin, config.user),
		Update:     osCommand.BuildCommandString(args.Update, args.Environment, args.Sudo, args.RequirePasswordFromStdin, config.user),
		Delete:     osCommand.BuildCommandString(args.Delete, args.Environment, args.Sudo, args.RequirePasswordFromStdin, config.user),
		Triggers:   args.Triggers,
		Stdin:      args.Stdin,
	}, nil
}

// Transformer is a function that can be used to modify the command name and args.
// Examples: swapping `args.Delete` with `args.Create`, or adding `args.Triggers`, or editing the name
type Transformer func(name string, args RunnerCommandArgs) (string, RunnerCommandArgs)

type RunnerConfiguration struct {
	user       string
	connection remote.ConnectionInput
}

type Command interface {
	pulumi.Resource

	StdoutOutput() pulumi.StringOutput
	StderrOutput() pulumi.StringOutput
}

type LocalCommand struct {
	*local.Command
}

type RemoteCommand struct {
	*remote.Command
}

var _ Command = &RemoteCommand{}
var _ Command = &LocalCommand{}

func (c *LocalCommand) StdoutOutput() pulumi.StringOutput {
	return c.Command.Stdout
}

func (c *LocalCommand) StderrOutput() pulumi.StringOutput {
	return c.Command.Stderr
}

func (c *RemoteCommand) StdoutOutput() pulumi.StringOutput {
	return c.Command.Stdout
}

func (c *RemoteCommand) StderrOutput() pulumi.StringOutput {
	return c.Command.Stderr
}

type Runner interface {
	Environment() config.Env
	Namer() namer.Namer
	Config() RunnerConfiguration
	OsCommand() OSCommand
	PulumiOptions() []pulumi.ResourceOption

	Command(name string, args RunnerCommandArgs, opts ...pulumi.ResourceOption) (Command, error)

	newCopyFile(name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error)
	newCopyToRemoteFile(name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error)
}

var _ Runner = &RemoteRunner{}
var _ Runner = &LocalRunner{}

type RemoteRunner struct {
	e           config.Env
	namer       namer.Namer
	waitCommand Command
	config      RunnerConfiguration
	osCommand   OSCommand
	options     []pulumi.ResourceOption
}

type RemoteRunnerArgs struct {
	ParentResource pulumi.Resource
	ConnectionName string
	Connection     remote.ConnectionInput
	ReadyFunc      ReadyFunc
	User           string
	OSCommand      OSCommand
}

func NewRemoteRunner(e config.Env, args RemoteRunnerArgs) (*RemoteRunner, error) {
	runner := &RemoteRunner{
		e:     e,
		namer: namer.NewNamer(e.Ctx(), "remote").WithPrefix(args.ConnectionName),
		config: RunnerConfiguration{
			connection: args.Connection,
			user:       args.User,
		},
		osCommand: args.OSCommand,
		options: []pulumi.ResourceOption{
			e.WithProviders(config.ProviderCommand),
		},
	}

	if args.ParentResource != nil {
		runner.options = append(runner.options, pulumi.Parent(args.ParentResource), pulumi.DeletedWith(args.ParentResource))
	}

	if args.ReadyFunc != nil {
		var err error
		runner.waitCommand, err = args.ReadyFunc(runner)
		if err != nil {
			return nil, err
		}
		runner.options = append(runner.options, utils.PulumiDependsOn(runner.waitCommand))
	}

	return runner, nil
}

func (r *RemoteRunner) Environment() config.Env {
	return r.e
}

func (r *RemoteRunner) Namer() namer.Namer {
	return r.namer
}

func (r *RemoteRunner) Config() RunnerConfiguration {
	return r.config
}

func (r *RemoteRunner) OsCommand() OSCommand {
	return r.osCommand
}

func (r *RemoteRunner) Command(name string, args RunnerCommandArgs, opts ...pulumi.ResourceOption) (Command, error) {
	if args.Arguments().Sudo && r.config.user != "" {
		r.e.Ctx().Log.Info(fmt.Sprintf("warning: running sudo command on a runner with user %s, discarding user", r.config.user), nil)
	}

	remoteArgs, err := toRemoteCommandArgs(args, r.config, r.osCommand)
	if err != nil {
		return nil, err
	}

	cmd, err := remote.NewCommand(r.e.Ctx(), r.namer.ResourceName("cmd", name), remoteArgs, utils.MergeOptions(r.options, opts...)...)

	if err != nil {
		return nil, err
	}

	return &RemoteCommand{cmd}, nil
}

func (r *RemoteRunner) newCopyFile(name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return r.osCommand.copyRemoteFile(r, name, localPath, remotePath, opts...)
}

func (r *RemoteRunner) newCopyToRemoteFile(name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return r.osCommand.copyRemoteFileV2(r, name, localPath, remotePath, opts...)
}

func (r *RemoteRunner) PulumiOptions() []pulumi.ResourceOption {
	return r.options
}

type LocalRunner struct {
	e         config.Env
	namer     namer.Namer
	config    RunnerConfiguration
	osCommand OSCommand
}

type LocalRunnerArgs struct {
	User      string
	OSCommand OSCommand
}

func NewLocalRunner(e config.Env, args LocalRunnerArgs) *LocalRunner {
	localRunner := &LocalRunner{
		e:         e,
		namer:     namer.NewNamer(e.Ctx(), "local"),
		osCommand: args.OSCommand,
		config: RunnerConfiguration{
			user: args.User,
		},
	}

	return localRunner
}

func (r *LocalRunner) Environment() config.Env {
	return r.e
}

func (r *LocalRunner) Namer() namer.Namer {
	return r.namer
}

func (r *LocalRunner) Config() RunnerConfiguration {
	return r.config
}

func (r *LocalRunner) OsCommand() OSCommand {
	return r.osCommand
}

func (r *LocalRunner) Command(name string, args RunnerCommandArgs, opts ...pulumi.ResourceOption) (Command, error) {
	opts = utils.MergeOptions[pulumi.ResourceOption](opts, r.e.WithProviders(config.ProviderCommand))
	localArgs, err := toLocalCommandArgs(args, r.config, r.osCommand)
	if err != nil {
		return nil, err
	}

	cmd, err := local.NewCommand(r.e.Ctx(), r.namer.ResourceName("cmd", name), localArgs, opts...)

	if err != nil {
		return nil, err
	}

	return &LocalCommand{cmd}, nil
}

func (r *LocalRunner) newCopyFile(name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return r.osCommand.copyLocalFile(r, name, localPath, remotePath, opts...)
}

func (r *LocalRunner) newCopyToRemoteFile(name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return r.newCopyFile(name, localPath, remotePath, opts...)
}

func (r *LocalRunner) PulumiOptions() []pulumi.ResourceOption {
	return []pulumi.ResourceOption{}
}
