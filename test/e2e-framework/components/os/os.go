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

// Interfaces used by OS components
type PackageManager interface {
	// Ensure ensures that a package is installed
	// checkBinary is a binary that should be checked before running the install command, if it is not empty it will first run the `command -v checkBinary` command and if it fails it will run the installCmd,
	// if it succeeds we consider the package is already installed
	Ensure(packageRef string, transform command.Transformer, checkBinary string, opts ...PackageManagerOption) (command.Command, error)
	EnsureUninstalled(packageRef string, transform command.Transformer, checkBinary string, opts ...PackageManagerOption) (command.Command, error)
}

func AllowUnsignedPackages(allow bool) PackageManagerOption {
	return func(pm *PackageManagerParams) error {
		pm.AllowUnsignedPackages = allow
		return nil
	}
}

func WithPulumiResourceOptions(opts ...pulumi.ResourceOption) PackageManagerOption {
	return func(pm *PackageManagerParams) error {
		pm.PulumiResourceOptions = opts
		return nil
	}
}

type PackageManagerOption = func(*PackageManagerParams) error

type PackageManagerParams struct {
	AllowUnsignedPackages bool
	PulumiResourceOptions []pulumi.ResourceOption
}

type ServiceManager interface {
	// EnsureStarted starts or restarts (may be stop+start depending on implementation) the service if already running
	EnsureRestarted(serviceName string, transform command.Transformer, opts ...pulumi.ResourceOption) (command.Command, error)
}

// FileManager needs to be added here as well instead of the command package

// OS is the high-level interface for an OS INSIDE Pulumi code
type OS interface {
	Descriptor() Descriptor

	Runner() command.Runner
	FileManager() *command.FileManager
	PackageManager() PackageManager
	ServiceManger() ServiceManager
}

var _ OS = &os{}

// os is a generic implementation of OS interface
type os struct {
	descriptor     Descriptor
	runner         command.Runner
	fileManager    *command.FileManager
	packageManager PackageManager
	serviceManager ServiceManager
}

func (o os) Descriptor() Descriptor {
	return o.descriptor
}

func (o os) Runner() command.Runner {
	return o.runner
}

func (o os) FileManager() *command.FileManager {
	return o.fileManager
}

func (o os) PackageManager() PackageManager {
	return o.packageManager
}

func (o os) ServiceManger() ServiceManager {
	return o.serviceManager
}

func NewOS(
	e config.Env,
	descriptor Descriptor,
	runner command.Runner,
) OS {
	switch descriptor.Family() {
	case LinuxFamily:
		return newLinuxOS(e, descriptor, runner)
	case WindowsFamily:
		return newWindowsOS(e, descriptor, runner)
	case MacOSFamily:
		return newMacOS(e, descriptor, runner)
	case UnknownFamily:
		fallthrough
	default:
		panic(fmt.Sprintf("unsupported OS family: %v", descriptor.Family()))
	}
}
