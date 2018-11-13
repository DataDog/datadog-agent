// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows,!integrationcmd
// +build cpython

package app

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func shellToIntegrationsCommand(cmd *cobra.Command, args []string) error {
	cmdline := ".\\integrations.exe"
	args = append([]string{"integration"}, args...)
	cmdargs := strings.Join(args, " ")
	fmt.Printf("%s %s", cmdline, cmdargs)
	intCmd := exec.Command(cmdline, args...)

	stdout, err := intCmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			fmt.Printf("stdin: ")
			fmt.Println(in.Text())
		}
	}()
	stderr, err := intCmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			fmt.Printf("stderr: ")
			fmt.Println(in.Text())
		}
	}()
	err = intCmd.Run()
	if err != nil {
		fmt.Printf(color.RedString(
			fmt.Sprintf("error running command: %v", err)))
	}
	return nil
}
func installTuf(cmd *cobra.Command, args []string) error {
	args = append([]string{"install"}, args...)
	return shellToIntegrationsCommand(cmd, args)
}

func removeTuf(cmd *cobra.Command, args []string) error {
	args = append([]string{"remove"}, args...)
	return shellToIntegrationsCommand(cmd, args)
}

func searchTuf(cmd *cobra.Command, args []string) error {
	args = append([]string{"search"}, args...)
	return shellToIntegrationsCommand(cmd, args)
}

func freeze(cmd *cobra.Command, args []string) error {
	args = append([]string{"freeze"}, args...)
	return shellToIntegrationsCommand(cmd, args)
}

func getTufConfigPath() string {
	return ""
}
