// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ec2

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// MacOSPoolArgs is the Pulumi-free subset of vmArgs the macOS pool provisioner needs:
// vmArgs and buildArgs are private to this package, and defaultVMArgs additionally
// requires an aws.Environment (itself bound to a *pulumi.Context, e.g. for
// InstanceProfile/Tenancy config) that the non-Pulumi provisioner doesn't have. This
// resolves only the caller-supplied VMOptions plus the macOS-specific defaulting
// vmargs.go's defaultVMArgs applies (default instance type by architecture); every
// other default (subnet, security groups, key pair, instance profile...) comes from
// pool.LaunchConfig instead.
type MacOSPoolArgs struct {
	OSInfo             *os.Descriptor
	InstanceType       string
	HostID             string
	UserData           string
	HTTPTokensRequired bool
	VolumeThroughput   int
}

// ResolveMacOSPoolArgs applies opts (as passed to ec2.WithEC2InstanceOptions) and
// returns the Pulumi-free subset needed to launch a macOS pool instance directly via
// the AWS SDK.
func ResolveMacOSPoolArgs(opts ...VMOption) (*MacOSPoolArgs, error) {
	args, err := buildArgs(opts...)
	if err != nil {
		return nil, err
	}

	if args.osInfo == nil {
		args.osInfo = &os.MacOSDefault
	}

	instanceType := args.instanceType
	if instanceType == "" || strings.HasPrefix(instanceType, "t3.") || strings.HasPrefix(instanceType, "t4g.") {
		if args.osInfo.Architecture == os.ARM64Arch {
			instanceType = "mac2.metal"
		} else {
			instanceType = "mac1.metal"
		}
	}

	return &MacOSPoolArgs{
		OSInfo:             args.osInfo,
		InstanceType:       instanceType,
		HostID:             args.hostID,
		UserData:           args.userData,
		HTTPTokensRequired: args.httpTokensRequired,
		VolumeThroughput:   args.volumeThroughput,
	}, nil
}
