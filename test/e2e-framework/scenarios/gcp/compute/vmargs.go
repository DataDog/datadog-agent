package compute

import (
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/os"
)

type vmArgs struct {
	osInfo       *os.Descriptor
	instanceType string
	imageName    string
}

type VMOption = func(*vmArgs) error

func newParams(options ...VMOption) (*vmArgs, error) {
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
