// +build linux windows

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/system-probe/app"
)

func subCommands() (commandNames []string) {
	for _, command := range app.SysprobeCmd.Commands() {
		commandNames = append(commandNames, append(command.Aliases, command.Name())...)
	}
	return
}

func setDefaultCommandIfNonePresent() {
	args := []string{os.Args[0], "run"}
	if len(os.Args) > 1 {
		potentialCommand := os.Args[1]
		if potentialCommand == "help" {
			return
		}

		for _, command := range subCommands() {
			if command == potentialCommand {
				return
			}
		}
		args = append(args, os.Args[1:]...)
	}
	os.Args = args
}
