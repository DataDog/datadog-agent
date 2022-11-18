// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python
// +build python

// Package integrations implements 'agent integration'.
package integrations

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

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
	integrationVersionScript = `
import pkg_resources
try:
	print(pkg_resources.get_distribution('%s').version)
except pkg_resources.DistributionNotFound:
	pass
`
	sitePackagesPathScript = "import site; print(site.getsitepackages()[0])"
)

var (
	datadogPkgNameRe      = regexp.MustCompile("datadog-.*")
	yamlFileNameRe        = regexp.MustCompile("[\\w_]+\\.yaml.*")
	wheelPackageNameRe    = regexp.MustCompile("Name: (\\S+)")                                                                                   // e.g. Name: datadog-postgres
	versionSpecifiersRe   = regexp.MustCompile("([><=!]{1,2})([0-9.]*)")                                                                         // Matches version specifiers defined in https://packaging.python.org/specifications/core-metadata/#requires-dist-multiple-use
	pep440VersionStringRe = regexp.MustCompile("^(?P<release>\\d+\\.\\d+\\.\\d+)(?:(?P<preReleaseType>[a-zA-Z]+)(?P<preReleaseNumber>\\d+)?)?$") // e.g. 1.3.4b1, simplified form of: https://www.python.org/dev/peps/pep-0440

	rootDir             string
	reqAgentReleasePath string
	constraintsPath     string
)

// cliParams are the command-line arguments for the sub-subcommands.
//
// Note that not all params are present for all sub-subcommands.
type cliParams struct {
	*command.GlobalParams

	// args are the positional command-line arguments
	args []string

	allowRoot          bool
	verbose            int
	useSysPython       bool
	versionOnly        bool
	localWheel         bool
	thirdParty         bool
	pythonMajorVersion string
	python2Path        string
	python3Path        string
}

func (cp *cliParams) python() *pythonRunner {
	var pyPath pythonPath
	if cp.useSysPython {
		pyPath = sysPythonPath()
	} else {
		pyPath = defaultPythonPath()
	}

	if cp.python2Path != "" {
		pyPath.py2 = cp.python2Path
	}
	if cp.python3Path != "" {
		pyPath.py3 = cp.python3Path
	}

	if cp.pythonMajorVersion == "2" {
		return python(pyPath.py2)
	} else {
		return python(pyPath.py3)
	}
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{}

	integrationCmd := &cobra.Command{
		Use:   "integration [command]",
		Short: "Datadog integration manager",
		Long:  ``,
	}

	integrationCmd.PersistentFlags().CountVarP(&cliParams.verbose, "verbose", "v", "enable verbose logging")
	integrationCmd.PersistentFlags().BoolVarP(&cliParams.allowRoot, "allow-root", "r", false, "flag to enable root to install packages")
	integrationCmd.PersistentFlags().BoolVarP(&cliParams.useSysPython, "use-sys-python", "p", false, "use system python instead [dev flag]")
	integrationCmd.PersistentFlags().StringVarP(&cliParams.pythonMajorVersion, "python", "", "", "the version of Python to act upon (2 or 3). defaults to the python_version setting in datadog.yaml")
	integrationCmd.PersistentFlags().StringVarP(&cliParams.python2Path, "python2-path", "", "", "use python executable at path as python2[dev flag]")
	integrationCmd.PersistentFlags().StringVarP(&cliParams.python3Path, "python3-path", "", "", "use python executable at path as python3[dev flag]")

	// Power user flags - mark as hidden
	integrationCmd.PersistentFlags().MarkHidden("use-sys-python") //nolint:errcheck
	integrationCmd.PersistentFlags().MarkHidden("python2-path")   //nolint:errcheck
	integrationCmd.PersistentFlags().MarkHidden("python3-path")   //nolint:errcheck

	// all subcommands use the same provided components, with a different oneShot callback
	runOneShot := func(callback interface{}) error {
		return fxutil.OneShot(callback,
			fx.Supply(cliParams),
			fx.Supply(core.BundleParams{
				ConfFilePath:      globalParams.ConfFilePath,
				ConfigLoadSecrets: false,
				ConfigMissingOK:   true,
			}),
			core.Bundle,
		)
	}

	installCmd := &cobra.Command{
		Use:   "install [package==version]",
		Short: "Install Datadog integration core/extra packages",
		Long: `Install Datadog integration core/extra packages
You must specify a version of the package to install using the syntax: <package>==<version>, with
 - <package> of the form datadog-<integration-name>
 - <version> of the form x.y.z`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			return runOneShot(install)
		},
	}
	installCmd.Flags().BoolVarP(
		&cliParams.localWheel, "local-wheel", "w", false, fmt.Sprintf("install an agent check from a locally available wheel file. %s", disclaimer),
	)
	installCmd.Flags().BoolVarP(
		&cliParams.thirdParty, "third-party", "t", false, "install a community or vendor-contributed integration",
	)
	integrationCmd.AddCommand(installCmd)

	removeCmd := &cobra.Command{
		Use:   "remove [package]",
		Short: "Remove Datadog integration core/extra packages",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			return runOneShot(remove)
		},
	}
	integrationCmd.AddCommand(removeCmd)

	freezeCmd := &cobra.Command{
		Use:   "freeze",
		Short: "Print the list of installed packages in the agent's python environment",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			return runOneShot(list)
		},
	}
	integrationCmd.AddCommand(freezeCmd)

	showCmd := &cobra.Command{
		Use:   "show [package]",
		Short: "Print out information about [package]",
		Args:  cobra.ExactArgs(1),
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			return runOneShot(show)
		},
	}
	showCmd.Flags().BoolVarP(&cliParams.versionOnly, "show-version-only", "q", false, "only display version information")
	integrationCmd.AddCommand(showCmd)

	return []*cobra.Command{integrationCmd}
}

func loadPythonInfo(config config.Component, cliParams *cliParams) error {
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

	if cliParams.pythonMajorVersion == "" {
		cliParams.pythonMajorVersion = config.GetString("python_version")
	}

	constraintsPath = filepath.Join(rootDir, fmt.Sprintf("final_constraints-py%s.txt", cliParams.pythonMajorVersion))
	if _, err := os.Lstat(constraintsPath); err != nil {
		return err
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

// PEP440ToSemver TODO <agent-integrations>
func PEP440ToSemver(pep440 string) (*semver.Version, error) {
	// Note: this is a simplified version that more closely fits how integrations are versioned.
	matches := pep440VersionStringRe.FindStringSubmatch(pep440)
	if matches == nil {
		return nil, fmt.Errorf("invalid format: %s", pep440)
	}
	versionString := matches[1]
	preReleaseType := matches[2]
	preReleaseNumber := matches[3]
	if preReleaseType != "" {
		if preReleaseNumber == "" {
			preReleaseNumber = "0"
		}
		switch preReleaseType {
		case "a":
			versionString = fmt.Sprintf("%s-alpha.%s", versionString, preReleaseNumber)
		case "b":
			versionString = fmt.Sprintf("%s-beta.%s", versionString, preReleaseNumber)
		default:
			versionString = fmt.Sprintf("%s-%s.%s", versionString, preReleaseType, preReleaseNumber)
		}
	}
	return semver.NewVersion(versionString)
}

// Ensures a single argument is given and that it's valid as a package in the given context
func validateArgs(args []string, local bool) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("Too many arguments")
	} else if len(args) == 0 {
		return "", fmt.Errorf("Missing package argument")
	}

	packageName := args[0]

	if !local {
		if !datadogPkgNameRe.MatchString(packageName) {
			return "", fmt.Errorf("invalid package name - this manager only handles datadog packages. Did you mean `datadog-%s`?", packageName)
		}
	} else
	// Validate the wheel we try to install exists
	if _, err := os.Stat(packageName); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("local wheel %s does not exist", packageName)
		} else {
			return "", fmt.Errorf("cannot read local wheel %s: %v", packageName, err)
		}
	}

	return packageName, nil
}

func pip(python *pythonRunner, cmd string, args []string, verbose int) error {
	implicitFlags := args[0:]
	implicitFlags = append(implicitFlags, "--disable-pip-version-check")
	args = []string{cmd}

	if verbose > 0 {
		args = append(args, fmt.Sprintf("-%s", strings.Repeat("v", verbose)))
	}

	// Append implicit flags to the *pip* command
	args = append(args, implicitFlags...)

	return python.runModule("pip", args)
}

func install(config config.Component, cliParams *cliParams) error {
	if err := loadPythonInfo(config, cliParams); err != nil {
		return err
	}

	err := validateUser(cliParams.allowRoot)
	if err != nil {
		return err
	}
	pkgName, err := validateArgs(cliParams.args, cliParams.localWheel)
	if err != nil {
		return err
	}

	pipArgs := []string{
		"--constraint", constraintsPath,
		// We don't use pip to download wheels, so we don't need a cache
		"--no-cache-dir",
		// Specify to not use any index since we won't/shouldn't download anything with pip anyway
		"--no-index",
		// Do *not* install dependencies by default. This is partly to prevent
		// accidental installation / updates of third-party dependencies from PyPI.
		"--no-deps",
	}

	if cliParams.localWheel {
		// Specific case when installing from locally available wheel
		// No compatibility verifications are performed, just install the wheel (with --no-deps still)
		// Verify that the wheel depends on `datadog-checks-base` to decide if it's an agent check or not
		wheelPath := pkgName

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
		if err := pip(cliParams.python(), "install", append(pipArgs, wheelPath), cliParams.verbose); err != nil {
			return fmt.Errorf("error installing wheel %s: %v", wheelPath, err)
		}

		// Move configuration files
		if err := moveConfigurationFilesOf(cliParams, integration); err != nil {
			fmt.Printf("Installed %s from %s\n", integration, wheelPath)
			return fmt.Errorf("Some errors prevented moving %s configuration files: %v", integration, err)
		}

		fmt.Println(color.GreenString(fmt.Sprintf(
			"Successfully completed the installation of %s", integration,
		)))

		return nil
	}

	// Additional verification for installation
	if len(strings.Split(pkgName, "==")) != 2 {
		return fmt.Errorf("you must specify a version to install with <package>==<version>")
	}

	intVer := strings.Split(pkgName, "==")
	integration := normalizePackageName(strings.TrimSpace(intVer[0]))
	if integration == "datadog-checks-base" {
		return fmt.Errorf("this command does not allow installing datadog-checks-base")
	}
	versionToInstall, err := semver.NewVersion(strings.TrimSpace(intVer[1]))
	if err != nil || versionToInstall == nil {
		return fmt.Errorf("unable to get version of %s to install: %v", integration, err)
	}
	currentVersion, found, err := installedVersion(cliParams.python(), integration)
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
	if cliParams.thirdParty {
		rootLayoutType = "extras"
	}

	// Download the wheel
	wheelPath, err := downloadWheel(cliParams.python(), integration, semverToPEP440(versionToInstall), rootLayoutType, cliParams.verbose)
	if err != nil {
		return fmt.Errorf("error when downloading the wheel for %s %s: %v", integration, versionToInstall, err)
	}

	// Verify datadog-checks-base is compatible with the requirements
	shippedBaseVersion, found, err := installedVersion(cliParams.python(), "datadog-checks-base")
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
	if err := pip(cliParams.python(), "install", append(pipArgs, wheelPath), cliParams.verbose); err != nil {
		return fmt.Errorf("error installing wheel %s: %v", wheelPath, err)
	}

	// Move configuration files
	if err := moveConfigurationFilesOf(cliParams, integration); err != nil {
		fmt.Printf("Installed %s %s", integration, versionToInstall)
		return fmt.Errorf("Some errors prevented moving %s configuration files: %v", integration, err)
	}

	fmt.Println(color.GreenString(fmt.Sprintf(
		"Successfully installed %s %s", integration, versionToInstall,
	)))
	return nil
}

func downloadWheel(python *pythonRunner, integration, version, rootLayoutType string, verbose int) (string, error) {
	args := []string{
		integration,
		"--version", version,
		"--type", rootLayoutType,
	}
	if verbose > 0 {
		args = append(args, fmt.Sprintf("-%s", strings.Repeat("v", verbose)))
	}

	// We do all of the following so that when we call our downloader, which will
	// in turn call in-toto, which will in turn call Python to inspect the wheel,
	// we will use our embedded Python.
	// First, get the current PATH as an array.
	pathArr := filepath.SplitList(os.Getenv("PATH"))
	// Get the directory of our embedded Python.
	pythonDir := filepath.Dir(python.path)
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

	// Proxy support
	proxies := pkgconfig.GetProxies()
	if proxies != nil {
		environ = append(environ,
			fmt.Sprintf("HTTP_PROXY=%s", proxies.HTTP),
			fmt.Sprintf("HTTPS_PROXY=%s", proxies.HTTPS),
			fmt.Sprintf("NO_PROXY=%s", strings.Join(proxies.NoProxy, ",")),
		)
	}

	// Now, while downloaderCmd itself won't use the new PATH, any child process,
	// such as in-toto subprocesses, will.
	python = python.withEnv(environ)

	lastLine := ""

	callback := func(line string) {
		lastLine = line
	}
	if err := python.runModuleWithCallback(downloaderModule, args, callback); err != nil {
		return "", err
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
	lines, err := os.ReadFile(reqAgentReleasePath)
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
func installedVersion(python *pythonRunner, integration string) (*semver.Version, bool, error) {
	validName, err := regexp.MatchString("^[0-9a-z_-]+$", integration)
	if err != nil {
		return nil, false, fmt.Errorf("Error validating integration name: %s", err)
	}
	if !validName {
		return nil, false, fmt.Errorf("Cannot get installed version of %s: invalid integration name", integration)
	}

	output, err := python.runCommand(fmt.Sprintf(integrationVersionScript, integration))
	if err != nil {
		return nil, false, err
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return nil, false, nil
	}

	version, err := PEP440ToSemver(outputStr)
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

func getRelChecksPath(cliParams *cliParams) (string, error) {
	sitePackagesPath, err := cliParams.python().runCommand(sitePackagesPathScript)
	if err != nil {
		return "", err
	}
	sitePackagesPath = strings.TrimSpace(string(sitePackagesPath))
	return filepath.Join(sitePackagesPath, "datadog_checks"), nil
}

func moveConfigurationFilesOf(cliParams *cliParams, integration string) error {
	confFolder := pkgconfig.Datadog.GetString("confd_path")
	check := getIntegrationName(integration)
	confFileDest := filepath.Join(confFolder, fmt.Sprintf("%s.d", check))
	if err := os.MkdirAll(confFileDest, os.ModeDir|0755); err != nil {
		return err
	}

	relChecksPath, err := getRelChecksPath(cliParams)
	if err != nil {
		return err
	}
	confFileSrc := filepath.Join(relChecksPath, check, "data")

	return moveConfigurationFiles(confFileSrc, confFileDest)
}

func moveConfigurationFiles(srcFolder string, dstFolder string) error {
	files, err := os.ReadDir(srcFolder)
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
		srcContent, err := os.ReadFile(src)
		if err != nil {
			errorMsg = fmt.Sprintf("%s\nError reading configuration file %s: %v", errorMsg, src, err)
			continue
		}
		err = os.WriteFile(dst, srcContent, 0644)
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

func remove(config config.Component, cliParams *cliParams) error {
	if err := loadPythonInfo(config, cliParams); err != nil {
		return err
	}

	err := validateUser(cliParams.allowRoot)
	if err != nil {
		return err
	}

	pkgName, err := validateArgs(cliParams.args, false)
	if err != nil {
		return err
	}

	pipArgs := []string{
		"--no-cache-dir",
	}
	pipArgs = append(pipArgs, pkgName)
	pipArgs = append(pipArgs, "-y")

	return pip(cliParams.python(), "uninstall", pipArgs, cliParams.verbose)
}

func list(config config.Component, cliParams *cliParams) error {
	if err := loadPythonInfo(config, cliParams); err != nil {
		return err
	}

	pipArgs := []string{
		"--format=freeze",
	}

	pipStdo := bytes.NewBuffer(nil)

	err := pip(cliParams.python(), "list", pipArgs, cliParams.verbose)
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

func show(config config.Component, cliParams *cliParams) error {
	if err := loadPythonInfo(config, cliParams); err != nil {
		return err
	}

	packageName, err := validateArgs(cliParams.args, false)
	if err != nil {
		return err
	}
	packageName = normalizePackageName(packageName)

	version, found, err := installedVersion(cliParams.python(), packageName)
	if err != nil {
		return fmt.Errorf("could not get current version of %s: %v", packageName, err)
	} else if !found {
		return fmt.Errorf("could not get current version of %s: not installed", packageName)
	}

	if version == nil {
		// Package not installed, return 0 and print nothing
		return nil
	}

	if cliParams.versionOnly {
		// Print only the version for easier parsing
		fmt.Println(version)
	} else {
		fmt.Printf("Package %s:\nInstalled version: %s\n", packageName, version)
	}

	return nil
}
