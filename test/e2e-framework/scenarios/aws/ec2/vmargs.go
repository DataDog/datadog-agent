// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2/pool"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Params defines the parameters for a virtual machine.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithOS]
//   - [WithAMI]
//   - [WithLatestAMI]
//   - [WithArch]
//   - [WithInstanceType]
//   - [WithUserData]
//   - [WithName]
//   - [WithHostID]
//   - [WithTenancy]
//   - [WithVolumeThroughput]
//   - [WithPulumiResourceOptions]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis

type vmArgs struct {
	osInfo          *os.Descriptor
	ami             string
	useLatestAMI    bool
	userData        string
	instanceType    string
	instanceProfile string
	tenancy         string
	hostID          string

	httpTokensRequired    bool
	volumeThroughput      int // GP3 volume throughput in MiB/s (125-1000, default 125)
	pulumiResourceOptions []pulumi.ResourceOption

	// preAcquiredPool, when set, is a pool.AcquireResult already claimed by the
	// caller (see WithPreAcquiredPoolResult) before NewVM ran -- e.g. by
	// awshost.Provisioner's pre-Up hook, ahead of the Pulumi stack lock. NewVM
	// then skips its own pool.NewEC2Client/pool.Acquire call and reuses this one.
	preAcquiredPool *PreAcquiredPoolResult
}

type VMOption = func(*vmArgs) error

func buildArgs(options ...VMOption) (*vmArgs, error) {
	vmArgs := &vmArgs{}
	vmArgs.pulumiResourceOptions = []pulumi.ResourceOption{}
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

// WithAMI sets the AMI directly, skipping resolve process. `supportedOS` and `arch` must match the AMI requirements.
func WithAMI(ami string, osDesc os.Descriptor, arch os.Architecture) VMOption {
	return func(p *vmArgs) error {
		p.osInfo = utils.Pointer(osDesc.WithArch(arch))
		p.ami = ami
		return nil
	}
}

// WithLatestAMI sets the latest AMI for the OS and architecture.
func WithLatestAMI() VMOption {
	return func(p *vmArgs) error {
		p.useLatestAMI = true
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

func WithInstanceProfile(instanceProfile string) VMOption {
	return func(p *vmArgs) error {
		p.instanceProfile = instanceProfile
		return nil
	}
}

func WithIMDSv1Disable() VMOption {
	return func(p *vmArgs) error {
		p.httpTokensRequired = true
		return nil
	}
}

// WithHostId sets the dedicated host ID for the instance
func WithHostID(hostID string) VMOption {
	return func(p *vmArgs) error {
		p.hostID = hostID
		return nil
	}
}

// WithTenancy sets the tenancy for the instance
func WithTenancy(tenancy string) VMOption {
	return func(p *vmArgs) error {
		p.tenancy = tenancy
		return nil
	}
}

func WithPulumiResourceOptions(options ...pulumi.ResourceOption) VMOption {
	return func(p *vmArgs) error {
		p.pulumiResourceOptions = options
		return nil
	}
}

// WithVolumeThroughput sets the throughput for the root GP3 volume in MiB/s.
// Valid range: 125-1000. Default is 125 MiB/s if not specified.
// This option only applies to GP3 volumes.
func WithVolumeThroughput(throughput int) VMOption {
	return func(p *vmArgs) error {
		p.volumeThroughput = throughput
		return nil
	}
}

// PreAcquiredPoolResult carries a pool.AcquireResult claimed by a caller ahead
// of NewVM -- e.g. by awshost.Provisioner's pre-Up hook, before the Pulumi
// stack lock is taken -- together with a flag NewVM flips to true right after
// it successfully hands ownership of releasing the lease over to
// ec2.ScheduleReleaseOnDestroy. The caller's own cleanup (e.g. releasing the
// lease if Up never got that far) must check this flag to avoid double-release.
type PreAcquiredPoolResult struct {
	Result  pool.AcquireResult
	Claimed *bool
}

// WithPreAcquiredPoolResult supplies a pool.AcquireResult already claimed by
// the caller, so NewVM reuses it instead of calling pool.NewEC2Client/pool.Acquire
// itself. Only meaningful for macOS pool members (see IsMacOSPoolCandidate);
// ignored otherwise.
func WithPreAcquiredPoolResult(r *PreAcquiredPoolResult) VMOption {
	return func(p *vmArgs) error {
		p.preAcquiredPool = r
		return nil
	}
}

// IsMacOSPoolCandidate reports whether the given VM options describe a macOS
// pool member (a macOS VM with no explicit dedicated HostID), the same
// condition NewVM uses to decide whether to draw from the pool. It only
// inspects pure option state via buildArgs -- no AMI-resolution network calls
// -- so it's safe to call before a *pulumi.Context exists, e.g. from a
// provisioner's pre-Up hook.
func IsMacOSPoolCandidate(opts ...VMOption) (bool, error) {
	vmArgs, err := buildArgs(opts...)
	if err != nil {
		return false, err
	}
	return vmArgs.osInfo != nil && vmArgs.osInfo.Family() == os.MacOSFamily && vmArgs.hostID == "", nil
}
