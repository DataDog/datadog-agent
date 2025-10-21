package localpodman

import (
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	componentsos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/DataDog/test-infra-definitions/resources/local"
	localpodman "github.com/DataDog/test-infra-definitions/resources/local/podman"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// NewVM creates an Ubuntu container instance on podman that emulates a VM and returns a Host component.
func NewVM(e local.Environment, name string) (*remote.Host, error) {
	// Create the EC2 instance
	return components.NewComponent(&e, e.Namer.ResourceName(name), func(c *remote.Host) error {
		vmArgs := &localpodman.VMArgs{
			Name: name,
		}

		// Create the EC2 instance
		address, user, port, err := localpodman.NewInstance(e, *vmArgs, pulumi.Parent(c))
		if err != nil {
			return err
		}

		// Create connection
		conn, err := remote.NewConnection(
			address,
			user,
			remote.WithPort(port),
		)
		if err != nil {
			return err
		}
		return remote.InitHost(&e, conn.ToConnectionOutput(), componentsos.Ubuntu2204, user, pulumi.String("").ToStringOutput(), command.WaitForSuccessfulConnection, c)
	})
}
