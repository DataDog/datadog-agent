// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package app

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	constraintsFile = "agent_requirements.txt"
	tufConfigFile   = "public-tuf-config.json"
	pyPiServer      = "https://pypi.org/simple/"
)

var (
	allowRoot    bool
	withoutTuf   bool
	useSysPython bool
	tufConfig    string
)

func init() {
	AgentCmd.AddCommand(tufCmd)
	tufCmd.AddCommand(installCmd)
	tufCmd.AddCommand(removeCmd)
	tufCmd.AddCommand(searchCmd)
	tufCmd.AddCommand(freezeCmd)
	tufCmd.PersistentFlags().BoolVarP(&withoutTuf, "no-tuf", "t", false, "don't use TUF repo")
	tufCmd.PersistentFlags().BoolVarP(&allowRoot, "allow-root", "r", false, "flag to enable root to install packages")
	tufCmd.PersistentFlags().BoolVarP(&useSysPython, "use-sys-python", "p", false, "use system python instead [dev flag]")
	tufCmd.PersistentFlags().StringVar(&tufConfig, "tuf-cfg", getTufConfigPath(), "path to TUF config file")
	tufCmd.PersistentFlags().StringSlice("cmd-flags", []string{}, "command flags to pass onto pip (comma-separated or multiple flags)")
	tufCmd.PersistentFlags().StringSlice("idx-flags", []string{}, "index flags to pass onto pip (comma-separated or multiple flags). "+
		"Some flags may not work with TUF enabled")

	// Power user flags - mark as hidden
	tufCmd.PersistentFlags().MarkHidden("cmd-flags")
	tufCmd.PersistentFlags().MarkHidden("idx-flags")
	tufCmd.PersistentFlags().MarkHidden("use-sys-python")
}

var tufCmd = &cobra.Command{
	Use:   "integration [command]",
	Short: "Datadog integration manager (ALPHA feature)",
	Long:  ``,
}

var installCmd = &cobra.Command{
	Use:   "install [package]",
	Short: "Install Datadog integration core/extra packages",
	Long:  ``,
	RunE:  installTuf,
}

var removeCmd = &cobra.Command{
	Use:   "remove [package]",
	Short: "Remove Datadog integration core/extra packages",
	Long:  ``,
	RunE:  removeTuf,
}

var searchCmd = &cobra.Command{
	Use:    "search [package]",
	Short:  "Search Datadog integration core/extra packages",
	Long:   ``,
	RunE:   searchTuf,
	Hidden: true,
}

var freezeCmd = &cobra.Command{
	Use:   "freeze",
	Short: "Freeze list of installed python packages",
	Long:  ``,
	RunE:  freeze,
}

func getTufConfigPath() string {
	here, _ := executable.Folder()
	return filepath.Join(here, relTufConfigFilePath)
}

func getCommandPython() (string, error) {
	if useSysPython {
		return pythonBin, nil
	}

	here, _ := executable.Folder()
	pyPath := filepath.Join(here, relPyPath)

	if _, err := os.Stat(pyPath); err != nil {
		if os.IsNotExist(err) {
			return pyPath, errors.New("unable to find pip executable")
		}
	}

	return pyPath, nil
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

	if flagNoColor {
		color.NoColor = true
	}

	err := common.SetupConfig(confFilePath)
	if err != nil {
		fmt.Printf("Cannot setup config, exiting: %v\n", err)
		return err
	}

	pipPath, err := getCommandPython()
	if err != nil {
		return err
	}
	tufPath, err := getTUFConfigFilePath()
	if err != nil && !withoutTuf {
		return err
	}

	cmd := args[0]
	implicitFlags := args[1:]
	implicitFlags = append(implicitFlags, "--disable-pip-version-check")
	args = append([]string{"-mpip"}, cmd)

	// Add pip power-user flags
	// cmd-flags go before the actual command
	cmdFlags, err := tufCmd.Flags().GetStringSlice("cmd-flags")
	if err == nil {
		args = append(args, cmdFlags...)
	}
	args = append(args, implicitFlags...)

	// Proxy support
	proxies := config.GetProxies()
	if proxies != nil {
		proxyFlags := fmt.Sprintf("--proxy=%s", proxies.HTTPS)
		args = append(args, proxyFlags)
	}

	// idx-flags go after the command and implicit flags
	idxFlags, err := tufCmd.Flags().GetStringSlice("idx-flags")
	if err == nil {
		args = append(args, idxFlags...)
	}

	tufCmd := exec.Command(pipPath, args...)

	if !withoutTuf {
		tufCmd.Env = append(os.Environ(),
			fmt.Sprintf("TUF_CONFIG_FILE=%s", tufPath),
		)
	}

	// forward the standard output to the Agent logger
	stdout, err := tufCmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			fmt.Println(in.Text())
		}
	}()

	// forward the standard error to the Agent logger
	stderr, err := tufCmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			fmt.Println(color.RedString(in.Text()))
		}
	}()

	err = tufCmd.Run()
	if err != nil {
		fmt.Printf(color.RedString(
			fmt.Sprintf("error running command: %v", err)))
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

	// NOTE: search will always go to pypi, Our TUF repository doesn't
	//       support searching currently.
	tufArgs := []string{
		"search",
	}
	tufArgs = append(tufArgs, args...)

	return tuf(tufArgs)
}

func freeze(cmd *cobra.Command, args []string) error {

	tufArgs := []string{
		"freeze",
	}
	tufArgs = append(tufArgs, args...)

	return tuf(tufArgs)
}
