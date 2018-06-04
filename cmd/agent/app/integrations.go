// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/spf13/cobra"
)

const (
	constraintsFile = "agent_requirements.txt"
	tufConfigFile   = "public-tuf-config.json"
	tufPyPiServer   = "https://integrations-core-wheels.s3.amazonaws.com/simple/"
	pyPiServer      = "https://pypi.org/simple/"
)

var (
	allowRoot bool
	withTuf   bool
	nativePkg bool
	tufConfig string
)

func init() {
	AgentCmd.AddCommand(tufCmd)
	tufCmd.AddCommand(installCmd)
	tufCmd.AddCommand(removeCmd)
	tufCmd.AddCommand(searchCmd)
	tufCmd.PersistentFlags().BoolVarP(&withTuf, "tuf", "t", true, "use TUF repo")
	tufCmd.PersistentFlags().BoolVarP(&nativePkg, "pip-package", "p", false, "providing native pip package name")
	tufCmd.PersistentFlags().BoolVarP(&allowRoot, "allow-root", "r", false, "flag to enable root to install packages")
	tufCmd.PersistentFlags().StringVar(&tufConfig, "tuf-cfg", getTufConfigPath(), "use TUF repo")
	tufCmd.PersistentFlags().StringSlice("cmd-flags", []string{}, "command flags to pass onto pip (comma-separated or multiple flags)")
	tufCmd.PersistentFlags().StringSlice("idx-flags", []string{}, "index flags to pass onto pip (comma-separated or multiple flags)")

	// Power user flags - mark as hidden
	tufCmd.Flags().MarkHidden("cmd-flags")
	tufCmd.Flags().MarkHidden("idx-flags")
}

var tufCmd = &cobra.Command{
	Use:   "integration [command]",
	Short: "Datadog integration/package manager",
	Long:  ``,
}

var installCmd = &cobra.Command{
	Use:   "install [package]",
	Short: "Install Datadog integration/extra packages",
	Long:  ``,
	RunE:  installTuf,
}

var removeCmd = &cobra.Command{
	Use:   "remove [package]",
	Short: "Remove Datadog integration/extra packages",
	Long:  ``,
	RunE:  removeTuf,
}

var searchCmd = &cobra.Command{
	Use:   "search [package]",
	Short: "Search Datadog integration/extra packages",
	Long:  ``,
	RunE:  searchTuf,
}

func getTufConfigPath() string {
	here, _ := executable.Folder()
	return filepath.Join(here, relTufConfigFilePath)
}

func getInstrumentedPipPath() (string, error) {
	here, _ := executable.Folder()
	pipPath := filepath.Join(here, relPipPath)

	if _, err := os.Stat(pipPath); err != nil {
		if os.IsNotExist(err) {
			return pipPath, errors.New("unable to find pip executable")
		}
	}

	return pipPath, nil
}

func getConstraintsFilePath() (string, error) {
	here, _ := executable.Folder()
	cPath := filepath.Join(here, relConstraintsPath)

	if _, err := os.Stat(cPath); err != nil {
		if os.IsNotExist(err) {
			return cPath, err
		}
	}

	return cPath, nil
}

func getTUFConfigFilePath() (string, error) {
	if _, err := os.Stat(tufConfig); err != nil {
		if os.IsNotExist(err) {
			return tufConfig, err
		}
	}

	return tufConfig, nil
}

func tuf(args []string) error {
	if !allowRoot && !authorizedUser() {
		return errors.New("Please use this tool as the agent-running user")
	}

	pipPath, err := getInstrumentedPipPath()
	if err != nil {
		return err
	}
	tufPath, err := getTUFConfigFilePath()
	if err != nil && withTuf {
		return err
	}

	// Add pip power-user flags
	// cmd-flags go before the actual command
	cmdFlags, err := tufCmd.Flags().GetStringSlice("cmd-flags")
	if err == nil {
		cmd := args[0]
		implicitFlags := args[1:]
		args = append([]string{cmd}, cmdFlags...)
		args = append(args, implicitFlags...)
	}

	// Proxy support
	proxies, err := config.GetProxies()
	if err == nil && proxies != nil {
		proxyFlags := fmt.Sprintf("--proxy=%s", proxies.HTTPS)
		args = append(args, proxyFlags)
	}

	// idx-flags go after the command and implicit flags
	idxFlags, err := tufCmd.Flags().GetStringSlice("idx-flags")
	if err == nil {
		args = append(args, idxFlags...)
	}

	tufCmd := exec.Command(pipPath, args...)

	var stdout, stderr bytes.Buffer
	tufCmd.Stdout = &stdout
	tufCmd.Stderr = &stderr
	if withTuf {
		tufCmd.Env = append(os.Environ(),
			fmt.Sprintf("TUF_CONFIG_FILE=%s", tufPath),
		)
	}

	err = tufCmd.Run()
	if err != nil {
		fmt.Printf("error running command: %v", stderr.String())
	} else {
		fmt.Printf("%v", stdout.String())
	}

	return err
}

func installTuf(cmd *cobra.Command, args []string) error {
	constraintsPath, err := getConstraintsFilePath()
	if err != nil {
		return err
	}

	cachePath, err := getTUFPipCachePath()
	if err != nil {
		return err
	}

	tufArgs := []string{
		"install",
		"--cache-dir", cachePath,
		"-c", constraintsPath,
	}

	tufArgs = append(tufArgs, args...)
	if withTuf {
		tufArgs = append(tufArgs, "--index-url", tufPyPiServer)
		tufArgs = append(tufArgs, "--extra-index-url", pyPiServer)
		tufArgs = append(tufArgs, "--disable-pip-version-check")
	}

	return tuf(tufArgs)
}

func removeTuf(cmd *cobra.Command, args []string) error {
	tufArgs := []string{
		"uninstall",
	}
	tufArgs = append(tufArgs, args...)
	tufArgs = append(tufArgs, "-y")

	return tuf(tufArgs)
}

func searchTuf(cmd *cobra.Command, args []string) error {

	tufArgs := []string{
		"search",
	}
	tufArgs = append(tufArgs, args...)
	if withTuf {
		tufArgs = append(tufArgs, "--index-url", tufPyPiServer)
		tufArgs = append(tufArgs, "--extra-index-url", pyPiServer)
		tufArgs = append(tufArgs, "--disable-pip-version-check")
	}

	return tuf(tufArgs)
}
