package remote

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// InitHost initializes all fields of a Host component with the given connection and OS descriptor.
func InitHost(e config.Env, conn remote.ConnectionOutput, osDesc os.Descriptor, osUser string, password pulumi.StringOutput, readyFunc command.ReadyFunc, host *Host) error {
	// Determine OSCommand implementation
	var osCommand command.OSCommand
	if osDesc.Family() == os.WindowsFamily {
		osCommand = command.NewWindowsOSCommand()
	} else {
		osCommand = command.NewUnixOSCommand()
	}

	// Now we can create the runner
	runner, err := command.NewRemoteRunner(e, command.RemoteRunnerArgs{
		ParentResource: host,
		ConnectionName: host.Name(),
		Connection:     conn,
		ReadyFunc:      readyFunc,
		OSCommand:      osCommand,
	})
	if err != nil {
		return err
	}

	// Fill the exported fields component
	host.Address = conn.Host()
	host.Username = pulumi.String(osUser).ToStringOutput()
	host.Architecture = pulumi.String(osDesc.Architecture).ToStringOutput()
	host.OSFamily = pulumi.Int(osDesc.Family()).ToIntOutput()
	host.OSFlavor = pulumi.Int(osDesc.Flavor).ToIntOutput()
	host.OSVersion = pulumi.String(osDesc.Version).ToStringOutput()
	host.Password = password
	host.Port = conn.Port().ApplyT(func(p *float64) int {
		if p == nil {
			// default port to 22
			return 22
		}
		return int(*p)
	}).(pulumi.IntOutput)

	// Set the OS for internal usage
	host.OS = os.NewOS(e, osDesc, runner)

	return nil
}
