// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package app

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
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
	tufConfigFile       = "public-tuf-config.json"
	reqAgentReleaseFile = "requirements-agent-release.txt"
	constraintsFile     = "final_constraints.txt"
	tufPkgPattern       = "datadog(-|_).*"
	tufIndex            = "https://dd-integrations-core-wheels-build-stable.s3.amazonaws.com/targets/simple/"
	reqLinePattern      = "%s==(\\d+\\.\\d+\\.\\d+)"
	yamlFilePattern     = "[\\w_]+\\.yaml.*"
	disclaimer          = "For your security, only use this to install wheels containing an Agent integration " +
		"and coming from a known source. The Agent cannot perform any verification on local wheels."
)

var (
	allowRoot    bool
	withoutTuf   bool
	inToto       bool
	verbose      bool
	useSysPython bool
	tufConfig    string
	versionOnly  bool
	localWheel   bool
)

type integrationVersion struct {
	major int
	minor int
	fix   int
}

// Parse a version string.
// Return the version, or nil empty string
func parseVersion(version string) (*integrationVersion, error) {
	var major, minor, fix int
	if version == "" {
		return nil, nil
	}
	_, err := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &fix)
	if err != nil {
		return nil, fmt.Errorf("unable to parse version string %s: %v", version, err)
	}
	return &integrationVersion{major, minor, fix}, nil
}

func (v *integrationVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.fix)
}

func (v *integrationVersion) isAboveOrEqualTo(otherVersion *integrationVersion) bool {
	if otherVersion == nil {
		return true
	}

	if v.major > otherVersion.major {
		return true
	} else if v.major < otherVersion.major {
		return false
	}

	if v.minor > otherVersion.minor {
		return true
	} else if v.minor < otherVersion.minor {
		return false
	}

	if v.fix > otherVersion.fix {
		return true
	} else if v.fix < otherVersion.fix {
		return false
	}

	return true
}

func (v *integrationVersion) equals(otherVersion *integrationVersion) bool {
	if otherVersion == nil {
		return false
	}

	return v.major == otherVersion.major && v.minor == otherVersion.minor && v.fix == otherVersion.fix
}

func init() {
	AgentCmd.AddCommand(integrationCmd)
	integrationCmd.AddCommand(installCmd)
	integrationCmd.AddCommand(removeCmd)
	integrationCmd.AddCommand(searchCmd)
	integrationCmd.AddCommand(freezeCmd)
	integrationCmd.AddCommand(showCmd)
	integrationCmd.PersistentFlags().BoolVarP(&withoutTuf, "no-tuf", "t", false, "don't use TUF repo")
	integrationCmd.PersistentFlags().BoolVarP(&inToto, "in-toto", "i", false, "enable in-toto")
	integrationCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging on pip and TUF")
	integrationCmd.PersistentFlags().BoolVarP(&allowRoot, "allow-root", "r", false, "flag to enable root to install packages")
	integrationCmd.PersistentFlags().BoolVarP(&useSysPython, "use-sys-python", "p", false, "use system python instead [dev flag]")
	integrationCmd.PersistentFlags().StringVar(&tufConfig, "tuf-cfg", getTufConfigPath(), "path to TUF config file")

	// Power user flags - mark as hidden
	integrationCmd.PersistentFlags().MarkHidden("use-sys-python")

	showCmd.Flags().BoolVarP(&versionOnly, "show-version-only", "q", false, "only display version information")
	installCmd.Flags().BoolVarP(
		&localWheel, "local-wheel", "w", false, fmt.Sprintf("install an agent check from a locally available wheel file. %s", disclaimer),
	)
}

var integrationCmd = &cobra.Command{
	Use:   "integration [command]",
	Short: "Datadog integration manager",
	Long:  ``,
}

var installCmd = &cobra.Command{
	Use:   "install [package==version]",
	Short: "Install Datadog integration core/extra packages",
	Long: `Install Datadog integration core/extra packages
You must specify a version of the package to install using the syntax: <package>==<version>, with
 - <package> of the form datadog-<integration-name>
 - <version> of the form x.y.z`,
	RunE: install,
}

var removeCmd = &cobra.Command{
	Use:   "remove [package]",
	Short: "Remove Datadog integration core/extra packages",
	Long:  ``,
	RunE:  remove,
}

var searchCmd = &cobra.Command{
	Use:    "search [package]",
	Short:  "Search Datadog integration core/extra packages",
	Long:   ``,
	RunE:   search,
	Hidden: true,
}

var freezeCmd = &cobra.Command{
	Use:   "freeze",
	Short: "Print the list of installed packages in the agent's python environment",
	Long:  ``,
	RunE:  freeze,
}

var showCmd = &cobra.Command{
	Use:   "show [package]",
	Short: "Print out information about [package]",
	Args:  cobra.ExactArgs(1),
	Long:  ``,
	RunE:  show,
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

func validateArgs(args []string, local bool) error {
	if len(args) > 1 {
		return fmt.Errorf("Too many arguments")
	} else if len(args) == 0 {
		return fmt.Errorf("Missing package argument")
	}

	if !local {
		exp, err := regexp.Compile(tufPkgPattern)
		if err != nil {
			return fmt.Errorf("internal error: %v", err)
		}

		if !exp.MatchString(args[0]) {
			return fmt.Errorf("invalid package name - this manager only handles datadog packages")
		}
	} else {
		// Validate the wheel we try to install exists
		if _, err := os.Stat(args[0]); err == nil {
			return nil
		} else if os.IsNotExist(err) {
			return fmt.Errorf("local wheel %s does not exist", args[0])
		} else {
			return fmt.Errorf("cannot read local wheel %s: %v", args[0], err)
		}
	}

	return nil
}

func pip(args []string, local bool) error {
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
	if err != nil && !withoutTuf && !local {
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

	pipCmd := exec.Command(pipPath, args...)
	pipCmd.Env = os.Environ()

	// Proxy support
	proxies := config.GetProxies()
	if proxies != nil {
		pipCmd.Env = append(pipCmd.Env,
			fmt.Sprintf("HTTP_PROXY=%s", proxies.HTTP),
			fmt.Sprintf("HTTPS_PROXY=%s", proxies.HTTPS),
			fmt.Sprintf("NO_PROXY=%s", strings.Join(proxies.NoProxy, ",")),
		)
	}

	if !withoutTuf && !local {
		pipCmd.Env = append(pipCmd.Env,
			fmt.Sprintf("TUF_CONFIG_FILE=%s", tufPath),
		)

		// Enable tuf logging
		if verbose {
			pipCmd.Env = append(pipCmd.Env,
				"TUF_ENABLE_LOGGING=1",
			)
		}

		// Enable phase 1, aka in-toto
		if inToto {
			pipCmd.Env = append(pipCmd.Env,
				"TUF_DOWNLOAD_IN_TOTO_METADATA=1",
			)
			// Change the working directory to one the Datadog Agent can read, so
			// that we can switch to temporary working directories, and back, for
			// in-toto.
			pipCmd.Dir, _ = executable.Folder()
		}
	} else {
		if inToto && withoutTuf {
			return errors.New("--in-toto conflicts with --no-tuf")
		} else if inToto && local {
			return errors.New("--in-toto conflicts with --local-wheel")
		}
	}

	// forward the standard output to the Agent logger
	stdout, err := pipCmd.StdoutPipe()
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
	stderr, err := pipCmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			fmt.Println(color.RedString(in.Text()))
		}
	}()

	err = pipCmd.Run()
	if err != nil {
		fmt.Printf(color.RedString(
			fmt.Sprintf("error running command: %v", err)))
	}

	return err
}

func install(cmd *cobra.Command, args []string) error {
	if err := validateArgs(args, localWheel); err != nil {
		return err
	}

	cachePath, err := getTUFPipCachePath()
	if err != nil {
		return err
	}

	here, err := executable.Folder()
	if err != nil {
		return err
	}
	constraintsPath := filepath.Join(here, relConstraintsPath)

	pipArgs := []string{
		"install",
		"--constraint", constraintsPath,
		"--cache-dir", cachePath,
		// We replace the PyPI index with our own by default, in order to prevent
		// accidental installation of Datadog or even third-party packages from
		// PyPI.
		"--index-url", tufIndex,
		// Do *not* install dependencies by default. This is partly to prevent
		// accidental installation / updates of third-party dependencies from PyPI.
		"--no-deps",
	}

	if localWheel {
		// Specific case when installing from locally available wheel
		// No compatibility verifications are performed, just install the wheel (with --no-deps still)
		// Verify that the wheel depends on `datadog_checks_base` to decide if it's an agent check or not
		fmt.Println(disclaimer)
		if ok, err := validateBaseDependency(args[0]); err != nil {
			return fmt.Errorf("error while reading the wheel %s: %v", args[0], err)
		} else if !ok {
			return fmt.Errorf("the wheel %s is not an agent check, it will not be installed", args[0])
		}
		return pip(append(pipArgs, args[0]), true)
	}

	// Additional verification for installation
	if len(strings.Split(args[0], "==")) != 2 {
		return fmt.Errorf("you must specify a version to install with <package>==<version>")
	}

	intVer := strings.Split(args[0], "==")
	integration := strings.Replace(strings.TrimSpace(intVer[0]), "_", "-", -1)
	if integration == "datadog-checks-base" {
		return fmt.Errorf("this command does not allow installing datadog-checks-base")
	}
	versionToInstall, err := parseVersion(strings.TrimSpace(intVer[1]))
	if err != nil || versionToInstall == nil {
		return fmt.Errorf("unable to get version of %s to install: %v", integration, err)
	}
	currentVersion, err := installedVersion(integration, cachePath)
	if err != nil {
		return fmt.Errorf("could not get current version of %s: %v", integration, err)
	}

	if versionToInstall.equals(currentVersion) {
		fmt.Printf("%s %s is already installed. Nothing to do.\n", integration, versionToInstall)
		return nil
	}

	minVersion, err := minAllowedVersion(integration)
	if err != nil {
		return fmt.Errorf("unable to get minimal version of %s: %v", integration, err)
	}
	if !versionToInstall.isAboveOrEqualTo(minVersion) {
		return fmt.Errorf(
			"this command does not allow installing version %s of %s older than version %s shipped with the agent",
			versionToInstall, integration, minVersion,
		)
	}

	// Run pip check first to see if the python environment is clean
	if err := pipCheck(cachePath); err != nil {
		return fmt.Errorf(
			"error when validating the agent's python environment, won't install %s: %v",
			integration, err,
		)
	}

	// Install the wheel
	if err := pip(append(pipArgs, args[0]), false); err != nil {
		return err
	}

	// Run pip check to determine if the installed integration is compatible with the base check version
	pipCheckErr := pipCheck(cachePath)
	if pipCheckErr == nil {
		err := moveConfigurationFilesOf(integration)
		if err == nil {
			fmt.Println(color.GreenString(fmt.Sprintf(
				"Successfully installed %s %s", integration, versionToInstall,
			)))
			return nil
		}

		fmt.Printf("Installed %s %s", integration, versionToInstall)
		return fmt.Errorf("Some errors prevented moving %s configuration files: %v", integration, err)
	}

	// We either detected a mismatch, or we failed to run pip check
	// Either way, roll back the install and return the error
	if currentVersion == nil {
		// Special case where we tried to install a new integration, not yet released with the agent
		pipArgs = []string{
			"uninstall",
			integration,
			"-y",
		}
	} else {
		pipArgs = append(pipArgs, fmt.Sprintf("%s==%s", integration, currentVersion))
	}

	// Perform the rollback
	pipErr := pip(pipArgs, false)
	if pipErr == nil {
		// Rollback successful, return error encountered during `pip check`
		return fmt.Errorf(
			"error when validating the agent's python environment, %s wasn't installed: %v",
			integration, pipCheckErr,
		)
	}

	// Rollback failed, mention that the integration could be broken
	return fmt.Errorf(
		"error when validating the agent's python environment, and the rollback failed, so %s %s was installed and might be broken:\n - %v\n- %v",
		integration, versionToInstall, pipCheckErr, pipErr,
	)
}

func validateBaseDependency(wheelPath string) (bool, error) {
	reader, err := zip.OpenReader(wheelPath)
	if err != nil {
		return false, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, "METADATA") {
			fileReader, err := file.Open()
			if err != nil {
				return false, err
			}
			defer fileReader.Close()
			scanner := bufio.NewScanner(fileReader)
			for scanner.Scan() {
				if strings.Contains(scanner.Text(), "Requires-Dist: datadog-checks-base") {
					return true, nil
				}
			}
			if err := scanner.Err(); err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

func minAllowedVersion(integration string) (*integrationVersion, error) {
	here, _ := executable.Folder()
	lines, err := ioutil.ReadFile(filepath.Join(here, relReqAgentReleasePath))
	if err != nil {
		return nil, err
	}
	version, err := getVersionFromReqLine(integration, string(lines))
	if err != nil {
		return nil, err
	}

	return version, nil
}

// Return the version of an installed integration, or nil if the integration isn't installed
func installedVersion(integration string, cachePath string) (*integrationVersion, error) {
	pythonPath, err := getCommandPython()
	if err != nil {
		return nil, err
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

		return nil, fmt.Errorf("error executing pip freeze: %s", errMsg)
	}

	version, err := getVersionFromReqLine(integration, string(output))
	if err != nil {
		return nil, err
	}

	return version, nil
}

// Parse requirements lines to get a package version.
// Returns the version if found, or nil if package not present
func getVersionFromReqLine(integration string, lines string) (*integrationVersion, error) {
	exp, err := regexp.Compile(fmt.Sprintf(reqLinePattern, integration))
	if err != nil {
		return nil, fmt.Errorf("internal error: %v", err)
	}

	groups := exp.FindAllStringSubmatch(lines, 2)
	if groups == nil {
		return nil, nil
	}

	if len(groups) > 1 {
		return nil, fmt.Errorf("Found several matches for %s version in %s\nAborting", integration, lines)
	}

	version, err := parseVersion(groups[0][1])
	if err != nil {
		return nil, err
	}
	return version, nil
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

func moveConfigurationFilesOf(integration string) error {
	confFolder := config.Datadog.GetString("confd_path")
	check := strings.TrimPrefix(integration, "datadog-")
	if check != "go-metro" {
		check = strings.Replace(check, "-", "_", -1)
	}
	confFileDest := filepath.Join(confFolder, fmt.Sprintf("%s.d", check))
	if err := os.MkdirAll(confFileDest, os.ModeDir|0755); err != nil {
		return err
	}

	here, _ := executable.Folder()
	confFileSrc := filepath.Join(here, relChecksPath, check, "data")
	return moveConfigurationFiles(confFileSrc, confFileDest)
}

func moveConfigurationFiles(srcFolder string, dstFolder string) error {
	files, err := ioutil.ReadDir(srcFolder)
	if err != nil {
		return err
	}

	exp, err := regexp.Compile(yamlFilePattern)
	if err != nil {
		return fmt.Errorf("internal error: %v", err)
	}
	errorMsg := ""
	for _, file := range files {
		filename := file.Name()
		// Replace existing file
		if !exp.MatchString(filename) {
			continue
		}
		src := filepath.Join(srcFolder, filename)
		dst := filepath.Join(dstFolder, filename)
		srcContent, err := ioutil.ReadFile(src)
		if err != nil {
			errorMsg = fmt.Sprintf("%s\nError reading configuration file %s: %v", errorMsg, src, err)
			continue
		}
		err = ioutil.WriteFile(dst, srcContent, 0644)
		if err != nil {
			errorMsg = fmt.Sprintf("%s\nError writing configuration file %s: %v", errorMsg, dst, err)
			continue
		}
		fmt.Println(color.GreenString(fmt.Sprintf(
			"Successfully copied configuration file %s", filename,
		)))
	}
	if errorMsg != "" {
		return fmt.Errorf(errorMsg)
	}
	return nil
}

func remove(cmd *cobra.Command, args []string) error {
	if err := validateArgs(args, false); err != nil {
		return err
	}

	pipArgs := []string{
		"uninstall",
	}
	pipArgs = append(pipArgs, args...)
	pipArgs = append(pipArgs, "-y")

	return pip(pipArgs, false)
}

func search(cmd *cobra.Command, args []string) error {

	// NOTE: search will always go to our TUF repository, which doesn't
	//       support searching currently.
	pipArgs := []string{
		"search",
		// We replace the PyPI index with our own by default, in order to prevent
		// accidental installation of Datadog or even third-party packages from
		// PyPI.
		"--index", tufIndex,
	}
	pipArgs = append(pipArgs, args...)

	return pip(pipArgs, false)
}

func freeze(cmd *cobra.Command, args []string) error {

	pipArgs := []string{
		"freeze",
	}

	return pip(pipArgs, false)
}

func show(cmd *cobra.Command, args []string) error {

	cachePath, err := getTUFPipCachePath()
	if err != nil {
		return err
	}

	packageName := strings.Replace(args[0], "_", "-", -1)

	version, err := installedVersion(packageName, cachePath)
	if err != nil {
		return fmt.Errorf("could not get current version of %s: %v", packageName, err)
	}

	if version == nil {
		// Package not installed, return 0 and print nothing
		return nil
	}

	if versionOnly {
		// Print only the version for easier parsing
		fmt.Println(version)
	} else {
		msg := `Package %s:
Installed version: %s
`
		fmt.Printf(msg, packageName, version)
	}

	return nil
}
