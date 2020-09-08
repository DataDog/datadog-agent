// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build python

package app

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"

	"github.com/coreos/go-semver/semver"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	reqAgentReleaseFile = "requirements-agent-release.txt"
	reqLinePattern      = "%s==(\\d+\\.\\d+\\.\\d+)"
	downloaderModule    = "datadog_checks.downloader"
	disclaimer          = "For your security, only use this to install wheels containing an Agent integration " +
		"and coming from a known source. The Agent cannot perform any verification on local wheels."
	pythonMinorVersionScript = "import sys;print(sys.version_info[1])"
	integrationVersionScript = `
import pkg_resources
try:
	print(pkg_resources.get_distribution('%s').version)
except pkg_resources.DistributionNotFound:
	pass
`
)

var (
	datadogPkgNameRe    = regexp.MustCompile("datadog-.*")
	yamlFileNameRe      = regexp.MustCompile("[\\w_]+\\.yaml.*")
	wheelPackageNameRe  = regexp.MustCompile("Name: (\\S+)")           // e.g. Name: datadog-postgres
	versionSpecifiersRe = regexp.MustCompile("([><=!]{1,2})([0-9.]*)") // Matches version specifiers defined in https://packaging.python.org/specifications/core-metadata/#requires-dist-multiple-use

	allowRoot           bool
	verbose             int
	useSysPython        bool
	versionOnly         bool
	localWheel          bool
	thirdParty          bool
	rootDir             string
	pythonMajorVersion  string
	pythonMinorVersion  string
	reqAgentReleasePath string
	constraintsPath     string
)

func init() {
	AgentCmd.AddCommand(integrationCmd)
	integrationCmd.AddCommand(installCmd)
	integrationCmd.AddCommand(removeCmd)
	integrationCmd.AddCommand(freezeCmd)
	integrationCmd.AddCommand(showCmd)
	integrationCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "enable verbose logging")
	integrationCmd.PersistentFlags().BoolVarP(&allowRoot, "allow-root", "r", false, "flag to enable root to install packages")
	integrationCmd.PersistentFlags().BoolVarP(&useSysPython, "use-sys-python", "p", false, "use system python instead [dev flag]")
	integrationCmd.PersistentFlags().StringVarP(&pythonMajorVersion, "python", "", "", "the version of Python to act upon (2 or 3). defaults to the python_version setting in datadog.yaml")

	// Power user flags - mark as hidden
	integrationCmd.PersistentFlags().MarkHidden("use-sys-python") //nolint:errcheck

	showCmd.Flags().BoolVarP(&versionOnly, "show-version-only", "q", false, "only display version information")
	installCmd.Flags().BoolVarP(
		&localWheel, "local-wheel", "w", false, fmt.Sprintf("install an agent check from a locally available wheel file. %s", disclaimer),
	)
	installCmd.Flags().BoolVarP(
		&thirdParty, "third-party", "t", false, "install a community or vendor-contributed integration",
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

var freezeCmd = &cobra.Command{
	Use:   "freeze",
	Short: "Print the list of installed packages in the agent's python environment",
	Long:  ``,
	RunE:  list,
}

var showCmd = &cobra.Command{
	Use:   "show [package]",
	Short: "Print out information about [package]",
	Args:  cobra.ExactArgs(1),
	Long:  ``,
	RunE:  show,
}

func loadPythonInfo() error {
	rootDir, _ = executable.Folder()
	for {
		agentReleaseFile := filepath.Join(rootDir, reqAgentReleaseFile)
		if _, err := os.Lstat(agentReleaseFile); err == nil {
			reqAgentReleasePath = agentReleaseFile
			break
		}

		parentDir := filepath.Dir(rootDir)
		if parentDir == rootDir {
			return fmt.Errorf("unable to locate %s", reqAgentReleaseFile)
		}

		rootDir = parentDir
	}

	if err := common.SetupConfig(confFilePath); err != nil {
		fmt.Printf("Cannot setup config, exiting: %v\n", err)
		return err
	}

	if pythonMajorVersion == "" {
		pythonMajorVersion = config.Datadog.GetString("python_version")
	}

	constraintsPath = filepath.Join(rootDir, fmt.Sprintf("final_constraints-py%s.txt", pythonMajorVersion))
	if _, err := os.Lstat(constraintsPath); err != nil {
		return err
	}

	return nil
}

func detectPythonMinorVersion() error {
	if pythonMinorVersion == "" {
		pythonPath, err := getCommandPython()
		if err != nil {
			return err
		}

		versionCmd := exec.Command(pythonPath, "-c", pythonMinorVersionScript)
		minorVersion, err := versionCmd.Output()
		if err != nil {
			return err
		}

		pythonMinorVersion = strings.TrimSpace(string(minorVersion))
	}

	return nil
}

func getIntegrationName(packageName string) string {
	switch packageName {
	case "datadog-checks-base":
		return "base"
	case "datadog-checks-downloader":
		return "downloader"
	case "datadog-go-metro":
		return "go-metro"
	default:
		return strings.TrimSpace(strings.Replace(strings.TrimPrefix(packageName, "datadog-"), "-", "_", -1))
	}
}

func normalizePackageName(packageName string) string {
	return strings.Replace(packageName, "_", "-", -1)
}

func semverToPEP440(version *semver.Version) string {
	pep440 := fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
	if version.PreRelease == "" {
		return pep440
	}
	parts := strings.SplitN(string(version.PreRelease), ".", 2)
	preReleaseType := parts[0]
	preReleaseNumber := ""
	if len(parts) == 2 {
		preReleaseNumber = parts[1]
	}
	switch preReleaseType {
	case "alpha":
		pep440 = fmt.Sprintf("%sa%s", pep440, preReleaseNumber)
	case "beta":
		pep440 = fmt.Sprintf("%sb%s", pep440, preReleaseNumber)
	default:
		pep440 = fmt.Sprintf("%src%s", pep440, preReleaseNumber)
	}
	return pep440
}

func getCommandPython() (string, error) {
	if useSysPython {
		return pythonBin, nil
	}

	pyPath := filepath.Join(rootDir, getRelPyPath())

	if _, err := os.Stat(pyPath); err != nil {
		if os.IsNotExist(err) {
			return pyPath, errors.New("unable to find python executable")
		}
	}

	return pyPath, nil
}

func validateArgs(args []string, local bool) error {
	if len(args) > 1 {
		return fmt.Errorf("Too many arguments")
	} else if len(args) == 0 {
		return fmt.Errorf("Missing package argument")
	}

	if !local {
		if !datadogPkgNameRe.MatchString(args[0]) {
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

func pip(args []string, stdout io.Writer, stderr io.Writer) error {
	if flagNoColor {
		color.NoColor = true
	}

	pythonPath, err := getCommandPython()
	if err != nil {
		return err
	}

	cmd := args[0]
	implicitFlags := args[1:]
	implicitFlags = append(implicitFlags, "--disable-pip-version-check")
	args = append([]string{"-mpip"}, cmd)

	if verbose > 0 {
		args = append(args, fmt.Sprintf("-%s", strings.Repeat("v", verbose)))
	}

	// Append implicit flags to the *pip* command
	args = append(args, implicitFlags...)

	pipCmd := exec.Command(pythonPath, args...)

	// forward the standard output to stdout
	pipStdout, err := pipCmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(pipStdout)
		for in.Scan() {
			fmt.Fprintf(stdout, "%s\n", in.Text())
		}
	}()

	// forward the standard error to stderr
	pipStderr, err := pipCmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(pipStderr)
		for in.Scan() {
			fmt.Fprintf(stderr, "%s\n", color.RedString(in.Text()))
		}
	}()

	err = pipCmd.Run()
	if err != nil {
		return fmt.Errorf("error running command: %v", err)
	}

	return nil
}

func install(cmd *cobra.Command, args []string) error {
	if err := loadPythonInfo(); err != nil {
		return err
	}

	err := validateUser(allowRoot)
	if err != nil {
		return err
	}

	if err := validateArgs(args, localWheel); err != nil {
		return err
	}

	pipArgs := []string{
		"install",
		"--constraint", constraintsPath,
		// We don't use pip to download wheels, so we don't need a cache
		"--no-cache-dir",
		// Specify to not use any index since we won't/shouldn't download anything with pip anyway
		"--no-index",
		// Do *not* install dependencies by default. This is partly to prevent
		// accidental installation / updates of third-party dependencies from PyPI.
		"--no-deps",
	}

	if localWheel {
		// Specific case when installing from locally available wheel
		// No compatibility verifications are performed, just install the wheel (with --no-deps still)
		// Verify that the wheel depends on `datadog-checks-base` to decide if it's an agent check or not
		wheelPath := args[0]

		fmt.Println(disclaimer)
		if ok, err := validateBaseDependency(wheelPath, nil); err != nil {
			return fmt.Errorf("error while reading the wheel %s: %v", wheelPath, err)
		} else if !ok {
			return fmt.Errorf("the wheel %s is not an agent check, it will not be installed", wheelPath)
		}

		// Parse the package name from metadata contained within the zip file
		integration, err := parseWheelPackageName(wheelPath)
		if err != nil {
			return err
		}
		integration = normalizePackageName(strings.TrimSpace(integration))

		// Install the wheel
		if err := pip(append(pipArgs, wheelPath), os.Stdout, os.Stderr); err != nil {
			return fmt.Errorf("error installing wheel %s: %v", wheelPath, err)
		}

		// Move configuration files
		if err := moveConfigurationFilesOf(integration); err != nil {
			fmt.Printf("Installed %s from %s\n", integration, wheelPath)
			return fmt.Errorf("Some errors prevented moving %s configuration files: %v", integration, err)
		}

		fmt.Println(color.GreenString(fmt.Sprintf(
			"Successfully completed the installation of %s", integration,
		)))

		return nil
	}

	// Additional verification for installation
	if len(strings.Split(args[0], "==")) != 2 {
		return fmt.Errorf("you must specify a version to install with <package>==<version>")
	}

	intVer := strings.Split(args[0], "==")
	integration := normalizePackageName(strings.TrimSpace(intVer[0]))
	if integration == "datadog-checks-base" {
		return fmt.Errorf("this command does not allow installing datadog-checks-base")
	}
	versionToInstall, err := semver.NewVersion(strings.TrimSpace(intVer[1]))
	if err != nil || versionToInstall == nil {
		return fmt.Errorf("unable to get version of %s to install: %v", integration, err)
	}
	currentVersion, found, err := installedVersion(integration)
	if err != nil {
		return fmt.Errorf("could not get current version of %s: %v", integration, err)
	}
	if found && versionToInstall.Equal(*currentVersion) {
		fmt.Printf("%s %s is already installed. Nothing to do.\n", integration, versionToInstall)
		return nil
	}

	minVersion, found, err := minAllowedVersion(integration)
	if err != nil {
		return fmt.Errorf("unable to get minimal version of %s: %v", integration, err)
	}
	if found && versionToInstall.LessThan(*minVersion) {
		return fmt.Errorf(
			"this command does not allow installing version %s of %s older than version %s shipped with the agent",
			versionToInstall, integration, minVersion,
		)
	}

	rootLayoutType := "core"
	if thirdParty {
		rootLayoutType = "extras"
	}

	// Download the wheel
	wheelPath, err := downloadWheel(integration, semverToPEP440(versionToInstall), rootLayoutType)
	if err != nil {
		return fmt.Errorf("error when downloading the wheel for %s %s: %v", integration, versionToInstall, err)
	}

	// Verify datadog-checks-base is compatible with the requirements
	shippedBaseVersion, found, err := installedVersion("datadog-checks-base")
	if err != nil {
		return fmt.Errorf("unable to get the version of datadog-checks-base: %v", err)
	}
	if ok, err := validateBaseDependency(wheelPath, shippedBaseVersion); found && err != nil {
		return fmt.Errorf("unable to validate compatibility of %s with the agent: %v", wheelPath, err)
	} else if !ok {
		return fmt.Errorf(
			"%s %s is not compatible with datadog-checks-base %s shipped in the agent",
			integration, versionToInstall, shippedBaseVersion,
		)
	}

	// Install the wheel
	if err := pip(append(pipArgs, wheelPath), os.Stdout, os.Stderr); err != nil {
		return fmt.Errorf("error installing wheel %s: %v", wheelPath, err)
	}

	// Move configuration files
	if err := moveConfigurationFilesOf(integration); err != nil {
		fmt.Printf("Installed %s %s", integration, versionToInstall)
		return fmt.Errorf("Some errors prevented moving %s configuration files: %v", integration, err)
	}

	fmt.Println(color.GreenString(fmt.Sprintf(
		"Successfully installed %s %s", integration, versionToInstall,
	)))
	return nil
}

func downloadWheel(integration, version, rootLayoutType string) (string, error) {
	pyPath, err := getCommandPython()
	if err != nil {
		return "", err
	}

	args := []string{
		"-m", downloaderModule,
		integration,
		"--version", version,
		"--type", rootLayoutType,
	}
	if verbose > 0 {
		args = append(args, fmt.Sprintf("-%s", strings.Repeat("v", verbose)))
	}

	downloaderCmd := exec.Command(pyPath, args...)

	// We do all of the following so that when we call our downloader, which will
	// in turn call in-toto, which will in turn call Python to inspect the wheel,
	// we will use our embedded Python.
	// First, get the current PATH as an array.
	pathArr := filepath.SplitList(os.Getenv("PATH"))
	// Get the directory of our embedded Python.
	pythonDir := filepath.Dir(pyPath)
	// Prepend this dir to PATH array.
	pathArr = append([]string{pythonDir}, pathArr...)
	// Build a new PATH string from the array.
	pathStr := strings.Join(pathArr, string(os.PathListSeparator))
	// Make a copy of the current environment.
	environ := os.Environ()
	// Walk over the copy of the environment, and replace PATH.
	for key, value := range environ {
		if strings.HasPrefix(value, "PATH=") {
			environ[key] = "PATH=" + pathStr
			// NOTE: Don't break so that we replace duplicate PATH-s, too.
		}
	}
	// Now, while downloaderCmd itself won't use the new PATH, any child process,
	// such as in-toto subprocesses, will.
	downloaderCmd.Env = environ

	// Proxy support
	proxies := config.GetProxies()
	if proxies != nil {
		downloaderCmd.Env = append(downloaderCmd.Env,
			fmt.Sprintf("HTTP_PROXY=%s", proxies.HTTP),
			fmt.Sprintf("HTTPS_PROXY=%s", proxies.HTTPS),
			fmt.Sprintf("NO_PROXY=%s", strings.Join(proxies.NoProxy, ",")),
		)
	}

	// forward the standard error to stderr
	stderr, err := downloaderCmd.StderrPipe()
	if err != nil {
		return "", err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			fmt.Fprintf(os.Stderr, "%s\n", color.RedString(in.Text()))
		}
	}()

	stdout, err := downloaderCmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := downloaderCmd.Start(); err != nil {
		return "", fmt.Errorf("error running command: %v", err)
	}
	lastLine := ""
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			lastLine = in.Text()
			fmt.Println(lastLine)
		}
	}()

	if err := downloaderCmd.Wait(); err != nil {
		return "", fmt.Errorf("error running command: %v", err)
	}

	// The path to the wheel will be at the last line of the output
	wheelPath := lastLine

	// Verify the availability of the wheel file
	if _, err := os.Stat(wheelPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("wheel %s does not exist", wheelPath)
		}
	}
	return wheelPath, nil
}

func parseWheelPackageName(wheelPath string) (string, error) {
	reader, err := zip.OpenReader(wheelPath)
	if err != nil {
		return "", fmt.Errorf("error operning archive file: %v", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, "METADATA") {
			fileReader, err := file.Open()
			if err != nil {
				return "", err
			}
			defer fileReader.Close()

			scanner := bufio.NewScanner(fileReader)
			for scanner.Scan() {
				line := scanner.Text()

				matches := wheelPackageNameRe.FindStringSubmatch(line)
				if matches == nil {
					continue
				}

				return matches[1], nil
			}
			if err := scanner.Err(); err != nil {
				return "", err
			}
		}
	}

	return "", fmt.Errorf("package name not found in wheel: %s", wheelPath)
}

func validateBaseDependency(wheelPath string, baseVersion *semver.Version) (bool, error) {
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
				line := scanner.Text()
				if strings.Contains(line, "Requires-Dist: datadog-checks-base") {
					if baseVersion == nil {
						// Simply trying to verify that the base package is a dependency
						return true, nil
					}
					matches := versionSpecifiersRe.FindAllStringSubmatch(line, -1)

					if matches == nil {
						// base check not pinned, so it is compatible with whatever version we pass
						return true, nil
					}

					compatible := true
					for _, groups := range matches {
						comp := groups[1]
						version, err := semver.NewVersion(groups[2])
						if err != nil {
							return false, fmt.Errorf("unable to parse version specifier %s in %s: %v", groups[0], line, err)
						}
						compatible = compatible && validateRequirement(baseVersion, comp, version)
					}
					return compatible, nil
				}
			}
			if err := scanner.Err(); err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

func validateRequirement(version *semver.Version, comp string, versionReq *semver.Version) bool {
	// Check for cases defined here: https://www.python.org/dev/peps/pep-0345/#version-specifiers
	switch comp {
	case "<": // version < versionReq
		return version.LessThan(*versionReq)
	case "<=": // version <= versionReq
		return !versionReq.LessThan(*version)
	case ">": // version > versionReq
		return versionReq.LessThan(*version)
	case ">=": // version >= versionReq
		return !version.LessThan(*versionReq)
	case "==": // version == versionReq
		return version.Equal(*versionReq)
	case "!=": // version != versionReq
		return !version.Equal(*versionReq)
	default:
		return false
	}
}

func minAllowedVersion(integration string) (*semver.Version, bool, error) {
	lines, err := ioutil.ReadFile(reqAgentReleasePath)
	if err != nil {
		return nil, false, err
	}
	version, found, err := getVersionFromReqLine(integration, string(lines))
	if err != nil {
		return nil, false, err
	}

	return version, found, nil
}

// Return the version of an installed integration and whether or not it was found
func installedVersion(integration string) (*semver.Version, bool, error) {
	pythonPath, err := getCommandPython()
	if err != nil {
		return nil, false, err
	}

	validName, err := regexp.MatchString("^[0-9a-z_-]+$", integration)
	if err != nil {
		return nil, false, fmt.Errorf("Error validating integration name: %s", err)
	}
	if !validName {
		return nil, false, fmt.Errorf("Cannot get installed version of %s: invalid integration name", integration)
	}

	pythonCmd := exec.Command(pythonPath, "-c", fmt.Sprintf(integrationVersionScript, integration))
	output, err := pythonCmd.Output()

	if err != nil {
		errMsg := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg = string(exitErr.Stderr)
		} else {
			errMsg = err.Error()
		}

		return nil, false, fmt.Errorf("error executing python: %s", errMsg)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return nil, false, nil
	}

	replacer := strings.NewReplacer("alpha", "-alpha", "beta", "-beta", "rc", "-rc")
	normalizedVersion := replacer.Replace(outputStr)

	version, err := semver.NewVersion(normalizedVersion)
	if err != nil {
		return nil, true, fmt.Errorf("error parsing version %s: %s", version, err)
	}

	return version, true, nil
}

// Parse requirements lines to get a package version.
// Returns the version and whether or not it was found
func getVersionFromReqLine(integration string, lines string) (*semver.Version, bool, error) {
	exp, err := regexp.Compile(fmt.Sprintf(reqLinePattern, integration))
	if err != nil {
		return nil, false, fmt.Errorf("internal error: %v", err)
	}

	groups := exp.FindAllStringSubmatch(lines, 2)
	if groups == nil {
		return nil, false, nil
	}

	if len(groups) > 1 {
		return nil, true, fmt.Errorf("Found several matches for %s version in %s\nAborting", integration, lines)
	}

	version, err := semver.NewVersion(groups[0][1])
	if err != nil {
		return nil, true, err
	}
	return version, true, nil
}

func moveConfigurationFilesOf(integration string) error {
	confFolder := config.Datadog.GetString("confd_path")
	check := getIntegrationName(integration)
	confFileDest := filepath.Join(confFolder, fmt.Sprintf("%s.d", check))
	if err := os.MkdirAll(confFileDest, os.ModeDir|0755); err != nil {
		return err
	}

	relChecksPath, err := getRelChecksPath()
	if err != nil {
		return err
	}
	confFileSrc := filepath.Join(rootDir, relChecksPath, check, "data")

	return moveConfigurationFiles(confFileSrc, confFileDest)
}

func moveConfigurationFiles(srcFolder string, dstFolder string) error {
	files, err := ioutil.ReadDir(srcFolder)
	if err != nil {
		return err
	}

	errorMsg := ""
	for _, file := range files {
		filename := file.Name()

		// Copy SNMP profiles
		if filename == "profiles" {
			profileDest := filepath.Join(dstFolder, "profiles")
			if err = os.MkdirAll(profileDest, 0755); err != nil {
				errorMsg = fmt.Sprintf("%s\nError creating directory for SNMP profiles %s: %v", errorMsg, profileDest, err)
				continue
			}
			profileSrc := filepath.Join(srcFolder, "profiles")
			if err = moveConfigurationFiles(profileSrc, profileDest); err != nil {
				errorMsg = fmt.Sprintf("%s\nError moving SNMP profiles from %s to %s: %v", errorMsg, profileSrc, profileDest, err)
				continue
			}
			continue
		}

		// Replace existing file
		if !yamlFileNameRe.MatchString(filename) {
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
	if err := loadPythonInfo(); err != nil {
		return err
	}

	err := validateUser(allowRoot)
	if err != nil {
		return err
	}

	if err := validateArgs(args, false); err != nil {
		return err
	}

	pipArgs := []string{
		"uninstall",
		"--no-cache-dir",
	}
	pipArgs = append(pipArgs, args...)
	pipArgs = append(pipArgs, "-y")

	return pip(pipArgs, os.Stdout, os.Stderr)
}

func list(cmd *cobra.Command, args []string) error {
	if err := loadPythonInfo(); err != nil {
		return err
	}

	pipArgs := []string{
		"list",
		"--format=freeze",
	}

	pipStdo := bytes.NewBuffer(nil)
	err := pip(pipArgs, io.Writer(pipStdo), os.Stderr)
	if err != nil {
		return err
	}

	pythonLibs := strings.Split(pipStdo.String(), "\n")

	// The agent integration freeze command should only show datadog packages and nothing else
	for i := range pythonLibs {
		if strings.HasPrefix(pythonLibs[i], "datadog-") {
			fmt.Println(pythonLibs[i])
		}
	}
	return nil
}

func show(cmd *cobra.Command, args []string) error {
	if err := loadPythonInfo(); err != nil {
		return err
	}

	if err := validateArgs(args, false); err != nil {
		return err
	}
	packageName := normalizePackageName(args[0])

	version, found, err := installedVersion(packageName)
	if err != nil {
		return fmt.Errorf("could not get current version of %s: %v", packageName, err)
	} else if !found {
		return fmt.Errorf("could not get current version of %s: not installed", packageName)
	}

	if version == nil {
		// Package not installed, return 0 and print nothing
		return nil
	}

	if versionOnly {
		// Print only the version for easier parsing
		fmt.Println(version)
	} else {
		fmt.Printf("Package %s:\nInstalled version: %s\n", packageName, version)
	}

	return nil
}
