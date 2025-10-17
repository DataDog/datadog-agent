package compute

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/DataDog/test-infra-definitions/resources/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/resources/gcp"
)

func NewVM(e gcp.Environment, name string, option ...VMOption) (*remote.Host, error) {
	params, paramsErr := newParams(option...)
	if paramsErr != nil {
		return nil, paramsErr
	}

	if err := defaultVMArgs(e, params); err != nil {
		return nil, err
	}

	imageInfo, err := resolveOS(e, params)
	if err != nil {
		return nil, err
	}

	return components.NewComponent(&e, name, func(h *remote.Host) error {
		h.CloudProvider = pulumi.String(components.CloudProviderGCP).ToStringOutput()
		vm, err := compute.NewLinuxInstance(e, e.Namer.ResourceName(name), imageInfo.name, params.instanceType, pulumi.Parent(h))
		if err != nil {
			return err
		}

		conn, err := remote.NewConnection(
			vm.NetworkInterfaces.Index(pulumi.Int(0)).NetworkIp().Elem(),
			"gce",
			remote.WithPrivateKeyPath(e.DefaultPrivateKeyPath()),
			remote.WithPrivateKeyPassword(e.DefaultPrivateKeyPassword()),
		)
		if err != nil {
			return err
		}

		return remote.InitHost(&e, conn.ToConnectionOutput(), *params.osInfo, "gce", pulumi.String("").ToStringOutput(), command.WaitForSuccessfulConnection, h)
	})
}

func defaultVMArgs(e gcp.Environment, vmArgs *vmArgs) error {
	if vmArgs.osInfo == nil {
		vmArgs.osInfo = &os.UbuntuDefault
	}

	if vmArgs.instanceType == "" {
		vmArgs.instanceType = e.DefaultInstanceType()
	}

	return nil
}

type imageInfo struct {
	name string
}

func resolveOS(e gcp.Environment, vmArgs *vmArgs) (imageInfo, error) {
	if vmArgs.imageName == "" {
		resolver, ok := imageResolvers[vmArgs.osInfo.Flavor]
		if !ok {
			return imageInfo{}, fmt.Errorf("unsupported OS flavor %v", vmArgs.osInfo.Flavor)
		}
		image, err := resolver(e, *vmArgs.osInfo)
		if err != nil {
			return imageInfo{}, err
		}
		return imageInfo{name: image}, nil
	}
	return imageInfo{name: vmArgs.imageName}, nil

}
