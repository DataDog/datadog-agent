// +build linux windows

package main

import (
	"fmt"
	"os"
	"strings"

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

func checkForDeprecatedFlags() {
	for i, a := range os.Args {
		if strings.HasPrefix(a, "-config") {
			fmt.Println("WARNING: `-config` argument is deprecated and will be removed in a future version. Please use `--config` instead.")
			os.Args[i] = "-" + os.Args[i]
		} else if strings.HasPrefix(a, "-pid") {
			fmt.Println("WARNING: `-pid` argument is deprecated and will be removed in a future version. Please use `--pid` instead.")
			os.Args[i] = "-" + os.Args[i]
		}
	}
}
