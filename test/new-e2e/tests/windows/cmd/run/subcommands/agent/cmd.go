// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package cmd implements the agent subcommands
package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	runCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/cmd/run/common"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

func installCmd(cmd *cobra.Command, args []string) error {
	host, err := runCommon.CreateRemoteHost(cmd)
	if err != nil {
		return err
	}
	outputDir := runCommon.GetOutputDir(cmd)

	// get agent package from env
	agentPackage, err := agent.GetPackageFromEnv()
	if err != nil {
		return err
	}

	msiArgs := []string{}
	if cmd.ArgsLenAtDash() >= 0 {
		msiArgs = append(msiArgs, args[cmd.ArgsLenAtDash():]...)
	}

	apiKey, _ := cmd.Flags().GetString("apikey")
	if apiKey != "" {
		msiArgs = append(msiArgs, fmt.Sprintf(`APIKEY="%s"`, apiKey))
	}

	remoteMSIPath, err := windowsCommon.GetTemporaryFile(host)
	if err != nil {
		return err
	}
	err = windowsCommon.PutOrDownloadFile(host, agentPackage.URL, remoteMSIPath)
	if err != nil {
		return err
	}

	strMsiArgs := strings.Join(msiArgs, " ")
	fmt.Printf("Installing agent on %s with username %s\n", host.Address, host.Username)
	fmt.Printf("  args: %s\n", strMsiArgs)
	err = windowsCommon.InstallMSI(host, remoteMSIPath, strMsiArgs, filepath.Join(outputDir, "agent-install.log"))
	if err != nil {
		return err
	}

	fmt.Println("Agent installed")
	return nil
}

func uninstallCmd(cmd *cobra.Command, _ []string) error {
	host, err := runCommon.CreateRemoteHost(cmd)
	if err != nil {
		return err
	}
	outputDir := runCommon.GetOutputDir(cmd)

	// get agent config dir from registry before uninstalling
	var removeConfig bool
	var configDir string
	if removeConfig, _ = cmd.Flags().GetBool("remove-config"); removeConfig {
		configDir, err = agent.GetConfigRootFromRegistry(host)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Uninstalling agent from %s with username %s\n", host.Address, host.Username)
	err = agent.UninstallAgent(host, filepath.Join(outputDir, "agent-uninstall.log"))
	if err != nil {
		return err
	}

	if removeConfig && configDir != "" {
		fmt.Println("Removing agent configuration files")
		err = host.RemoveAll(configDir)
		if err != nil {
			return err
		}
	}

	fmt.Println("Agent uninstalled")
	return nil
}

func cmdCmd(cmd *cobra.Command, args []string) error {
	host, err := runCommon.CreateRemoteHost(cmd)
	if err != nil {
		return err
	}

	// Get agent install path from registry
	installRoot, err := agent.GetInstallPathFromRegistry(host)
	if err != nil {
		return err
	}
	agentCmd := fmt.Sprintf(`& '%s'`, filepath.Join(installRoot, "bin", "agent.exe"))
	cmdArgs := strings.Join(args[cmd.ArgsLenAtDash():], " ")
	cmdline := fmt.Sprintf("%s %s", agentCmd, cmdArgs)
	fmt.Printf("Running agent command: %s\n", cmdline)

	out, err := host.Execute(cmdline)
	if err != nil {
		return err
	}

	fmt.Print(out)
	return nil
}

// Init initializes the agent subcommands
func Init(rootCmd *cobra.Command) {
	var agentCmd = &cobra.Command{
		Use:   "agent",
		Short: "Agent commands",
		Long:  `Agent commands`,
	}

	// uninstall subcommand
	var uninstallCmd = &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the agent",
		Long:  `Uninstall the agent`,
		RunE:  uninstallCmd,
	}
	uninstallCmd.Flags().Bool("remove-config", false, "Remove the agent configuration files")

	// install subcommand
	var installCmd = &cobra.Command{
		Use:     "install",
		Short:   "Install the agent",
		Long:    `Install the agent`,
		Example: `WINDOWS_AGENT_VERSION=7.51.0-1 ... agent install -- 'TAGS="key_1:val_1,key_2:val_2"'`,
		RunE:    installCmd,
	}
	// set a default b/c trace-agent will fail without an api key
	installCmd.Flags().String("apikey", "00000000000000000000000000000000", "API key to use for the install")

	// cmd subcommand
	var cmdCmd = &cobra.Command{
		Use:     "command -- [args]...",
		Aliases: []string{"cmd"},
		Short:   "Run an agent subcommand",
		Long:    "Run an agent subcommand",
		Example: `  agent cmd -- status`,
		RunE:    cmdCmd,
	}

	agentCmd.AddCommand(uninstallCmd)
	agentCmd.AddCommand(installCmd)
	agentCmd.AddCommand(cmdCmd)

	rootCmd.AddCommand(agentCmd)
}
