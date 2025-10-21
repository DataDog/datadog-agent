package ec2

import (
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/os"
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
	pulumiResourceOptions []pulumi.ResourceOption
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
