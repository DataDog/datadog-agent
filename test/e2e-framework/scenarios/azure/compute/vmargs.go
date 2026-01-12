// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compute

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Params defines the parameters for a virtual machine.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithOS]
//   - [WithImageName]
//   - [WithArch]
//   - [WithInstanceType]
//   - [WithUserData]
//   - [WithName]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis

type vmArgs struct {
	osInfo                *os.Descriptor
	imageURN              string
	userData              string
	instanceType          string
	pulumiResourceOptions []pulumi.ResourceOption
}

type VMOption = func(*vmArgs) error

func buildArgs(options ...VMOption) (*vmArgs, error) {
	vmArgs := &vmArgs{}
	return common.ApplyOption(vmArgs, options)
}

// WithOS sets the OS
// Version defaults to latest
func WithOS(osDesc os.Descriptor) VMOption {
	return WithOSArch(osDesc, osDesc.Architecture)
}

// WithArch set the architecture and the operating system.
// Version defaults to latest
func WithOSArch(osDesc os.Descriptor, arch os.Architecture) VMOption {
	return func(p *vmArgs) error {
		p.osInfo = utils.Pointer(osDesc.WithArch(arch))
		return nil
	}
}

// WithImageURN sets the image URN directly, skipping resolve process.
func WithImageURN(imageURN string, osDesc os.Descriptor, arch os.Architecture) VMOption {
	return func(p *vmArgs) error {
		p.osInfo = utils.Pointer(osDesc.WithArch(arch))
		p.imageURN = imageURN
		return nil
	}
}

// WithInstanceType set the instance type
func WithInstanceType(instanceType string) VMOption {
	return func(p *vmArgs) error {
		p.instanceType = instanceType
		return nil
	}
}

// WithUserData set the userdata for the instance. User data contains commands that are run at the startup of the instance.
func WithUserData(userData string) VMOption {
	return func(p *vmArgs) error {
		p.userData = userData
		return nil
	}
}

// WithPulumiResourceOptions sets the pulumi.ResourceOptions for the VM
func WithPulumiResourceOptions(opts ...pulumi.ResourceOption) VMOption {
	return func(p *vmArgs) error {
		p.pulumiResourceOptions = append(p.pulumiResourceOptions, opts...)
		return nil
	}
}
