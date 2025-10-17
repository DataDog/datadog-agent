package command

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ReadyFunc func(Runner) (Command, error)

func WaitForCloudInit(runner Runner) (Command, error) {
	return runner.Command(
		"wait-cloud-init",
		&Args{
			// `sudo` is required for amazon linux
			Create: pulumi.String("cloud-init status --wait"),
			Sudo:   true,
		})
}

func WaitForSuccessfulConnection(runner Runner) (Command, error) {
	return runner.Command(
		"wait-successful-connection",
		&Args{
			// echo works in shell and powershell
			Create: pulumi.String("echo \"OK\""),
		})
}
