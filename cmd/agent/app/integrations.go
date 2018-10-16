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
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	tufConfigFile          = "public-tuf-config.json"
	tufPkgPattern          = "datadog-.*"
	tufIndex               = "https://dd-integrations-core-wheels-build-stable.s3.amazonaws.com/targets/simple/"
	pipFreezeOutputPattern = "%s==(\\d+\\.\\d+\\.\\d+)"
)

var (
	allowRoot    bool
	withoutTuf   bool
	inToto       bool
	verbose      bool
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
	tufCmd.PersistentFlags().BoolVarP(&inToto, "in-toto", "i", false, "enable in-toto")
	tufCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging on pip and TUF")
	tufCmd.PersistentFlags().BoolVarP(&allowRoot, "allow-root", "r", false, "flag to enable root to install packages")
	tufCmd.PersistentFlags().BoolVarP(&useSysPython, "use-sys-python", "p", false, "use system python instead [dev flag]")
	tufCmd.PersistentFlags().StringVar(&tufConfig, "tuf-cfg", getTufConfigPath(), "path to TUF config file")

	// Power user flags - mark as hidden
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
	Long: `Install Datadog integration core/extra packages
You must specify a version of the package to install using the syntax: <package>==<version>, with
 - <package> of the form datadog-<integration-name>
 - <version> of the form x.y.z`,
	RunE: installTuf,
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

func getTUFConfigFilePath() (string, error) {
	if _, err := os.Stat(tufConfig); err != nil {
		if os.IsNotExist(err) {
			return tufConfig, err
		}
	}

	return tufConfig, nil
}

func validateTufArgs(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Too many arguments")
	} else if len(args) == 0 {
		return fmt.Errorf("Missing package argument")
	}

	exp, err := regexp.Compile(tufPkgPattern)
	if err != nil {
		return fmt.Errorf("internal error: %v", err)
	}

	if !exp.MatchString(args[0]) {
		return fmt.Errorf("invalid package name - this manager only handles datadog packages")
	}

	return nil
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

	if verbose {
		args = append(args, "-vvv")
	}

	// Append implicit flags to the *pip* command
	args = append(args, implicitFlags...)

	tufCmd := exec.Command(pipPath, args...)
	tufCmd.Env = os.Environ()

	// Proxy support
	proxies := config.GetProxies()
	if proxies != nil {
		tufCmd.Env = append(tufCmd.Env,
			fmt.Sprintf("HTTP_PROXY=%s", proxies.HTTP),
			fmt.Sprintf("HTTPS_PROXY=%s", proxies.HTTPS),
			fmt.Sprintf("NO_PROXY=%s", strings.Join(proxies.NoProxy, ",")),
		)
	}

	if !withoutTuf {
		tufCmd.Env = append(tufCmd.Env,
			fmt.Sprintf("TUF_CONFIG_FILE=%s", tufPath),
		)

		// Enable tuf logging
		if verbose {
			tufCmd.Env = append(tufCmd.Env,
				"TUF_ENABLE_LOGGING=1",
			)
		}

		// Enable phase 1, aka in-toto
		if inToto {
			tufCmd.Env = append(tufCmd.Env,
				"TUF_DOWNLOAD_IN_TOTO_METADATA=1",
			)
		}
	} else {
		if inToto {
			return errors.New("--in-toto conflicts with --no-tuf")
		}
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
	if err := validateTufArgs(args); err != nil {
		return err
	}

	// Additional verification for installation
	if len(strings.Split(args[0], "==")) != 2 {
		return fmt.Errorf("you must specify a version to install with <package>==<version>")
	}

	cachePath, err := getTUFPipCachePath()
	if err != nil {
		return err
	}

	intVer := strings.Split(args[0], "==")
	integration := strings.TrimSpace(intVer[0])
	if integration == "datadog-checks-base" {
		return fmt.Errorf("cannot upgrade datadog-checks-base")
	}
	versionToInstall := strings.TrimSpace(intVer[1])
	currentVersion, err := getIntegrationVersion(integration, cachePath)
	if err != nil {
		return fmt.Errorf("could not get current version of %s: %v", integration, err)
	}

	// Run pip check first to see if the python environment is clean
	if err := pipCheck(cachePath); err != nil {
		return fmt.Errorf(
			"error when validating the agent's python environment, won't install %s: %v",
			integration, err,
		)
	}

	tufArgs := []string{
		"install",
		"--cache-dir", cachePath,
		// We replace the PyPI index with our own by default, in order to prevent
		// accidental installation of Datadog or even third-party packages from
		// PyPI.
		"--index-url", tufIndex,
		// Do *not* install dependencies by default. This is partly to prevent
		// accidental installation / updates of third-party dependencies from PyPI.
		"--no-deps",
	}

	// Install the wheel
	if err := tuf(append(tufArgs, args[0])); err != nil {
		return err
	}

	// Run pip check to determine if the installed integration is compatible with the base check version
	pipErr := pipCheck(cachePath)
	if pipErr == nil {
		fmt.Println(color.GreenString(fmt.Sprintf(
			"Successfully installed %s %s", integration, versionToInstall,
		)))
		return nil
	}

	// We either detected a mismatch, or we failed to run pip check
	// Either way, roll back the install and return the error
	if currentVersion == "" {
		// Special case where we tried to install a new integration, not yet released with the agent
		tufArgs = []string{
			"uninstall",
			integration,
			"-y",
		}
	} else {
		tufArgs = append(tufArgs, fmt.Sprintf("%s==%s", integration, currentVersion))
	}

	// Perform the rollback
	tufErr := tuf(tufArgs)
	if tufErr == nil {
		// Rollback successful, return error encountered during `pip check`
		return fmt.Errorf(
			"error when validating the agent's python environment, %s wasn't installed: %v",
			integration, err,
		)
	}

	// Rollback failed, mention that the integration could be broken
	return fmt.Errorf(
		"error when validating the agent's python environment, and the rollback failed, so %s %s was installed and might be broken: %v",
		integration, versionToInstall, err,
	)
}

func getIntegrationVersion(integration string, cachePath string) (string, error) {
	pythonPath, err := getCommandPython()
	if err != nil {
		return "", err
	}

	freezeCmd := exec.Command(pythonPath, "-mpip", "freeze", "--cache-dir", cachePath)
	output, err := freezeCmd.Output()

	if err != nil {
		errMsg := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg = string(exitErr.Stderr)
		} else {
			errMsg = err.Error()
		}

		return "", fmt.Errorf("error executing pip freeze: %s", errMsg)
	}

	exp, err := regexp.Compile(fmt.Sprintf(pipFreezeOutputPattern, integration))
	if err != nil {
		return "", fmt.Errorf("internal error: %v", err)
	}

	if groups := exp.FindStringSubmatch(string(output)); groups != nil {
		return groups[1], nil
	}

	return "", nil
}

func pipCheck(cachePath string) error {
	pythonPath, err := getCommandPython()
	if err != nil {
		return err
	}

	checkCmd := exec.Command(pythonPath, "-mpip", "check", "--cache-dir", cachePath)
	output, err := checkCmd.CombinedOutput()

	if err == nil {
		// Clean python environment
		return nil
	}

	if _, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("error executing pip check: %v", string(output))
	}

	return fmt.Errorf("error executing pip check: %v", err)
}

func removeTuf(cmd *cobra.Command, args []string) error {
	if err := validateTufArgs(args); err != nil {
		return err
	}

	tufArgs := []string{
		"uninstall",
	}
	tufArgs = append(tufArgs, args...)
	tufArgs = append(tufArgs, "-y")

	return tuf(tufArgs)
}

func searchTuf(cmd *cobra.Command, args []string) error {

	// NOTE: search will always go to our TUF repository, which doesn't
	//       support searching currently.
	tufArgs := []string{
		"search",
		// We replace the PyPI index with our own by default, in order to prevent
		// accidental installation of Datadog or even third-party packages from
		// PyPI.
		"--index", tufIndex,
	}
	tufArgs = append(tufArgs, args...)

	return tuf(tufArgs)
}

func freeze(cmd *cobra.Command, args []string) error {

	tufArgs := []string{
		"freeze",
	}

	return tuf(tufArgs)
}
