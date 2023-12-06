package integrations

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/Masterminds/semver"
)

type integrationUpgrade struct {
	Name           string `json:"name"`
	ID             string `json:"id"`
	Version        string `json:"version"`
	RootLayoutType string `json:"root_layout_type"`
}

const (
	downloaderModule            = "datadog_checks.downloader"
	integrationVersionScriptPy3 = `
from importlib.metadata import version, PackageNotFoundError
try:
	print(version('%s'))
except PackageNotFoundError:
	pass
`
)

var (
	pep440VersionStringRe = regexp.MustCompile(`^(?P<release>\d+\.\d+\.\d+)(?:(?P<preReleaseType>[a-zA-Z]+)(?P<preReleaseNumber>\d+)?)?$`) // e.g. 1.3.4b1, simplified form of: https://www.python.org/dev/peps/pep-0440
	yamlFileNameRe        = regexp.MustCompile(`[\w_]+\.yaml.*`)
)

func IntegrationsInstallCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	for configPath, originalConfig := range updates {
		if !strings.HasPrefix(originalConfig.Metadata.ID, "integration_upgrade_") {
			// We only care about integration upgrades
			log.Debugf("Ignoring integration upgrade %s", originalConfig.Metadata.ID)
			continue
		} else if originalConfig.Metadata.ApplyStatus.State != state.ApplyStateUnknown {
			log.Debugf("Ignoring integration upgrade %s, already processed", originalConfig.Metadata.ID)
			continue
		}

		var upgrade integrationUpgrade
		err := json.Unmarshal(originalConfig.Config, &upgrade)
		if err != nil {
			log.Errorf("Unexpected error while unmarshalling integration upgrade: %s", err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		// Mark it as unack first
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateUnacknowledged,
		})

		log.Infof("Received integration upgrade for %s: %s", upgrade.Name, upgrade.Version)

		// Check the in-toto layout is valid
		if upgrade.RootLayoutType != "core" && upgrade.RootLayoutType != "extras" {
			log.Errorf("Unexpected root layout type: %s", upgrade.RootLayoutType)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: fmt.Sprintf("Unexpected root layout type: %s", upgrade.RootLayoutType),
			})
			continue
		}

		// Check the version
		versionToInstall, err := semver.NewVersion(strings.TrimSpace(upgrade.Version))
		if err != nil {
			log.Errorf("Unexpected error while parsing integration version: %s", err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		// Get current version of the integration
		currentVersion, found, err := installedVersion(upgrade.Name)
		if err != nil {
			log.Errorf("Unexpected error while getting installed version: %s", err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		// Check min allowed version
		if found && currentVersion.Equal(versionToInstall) {
			log.Infof("Integration %s already at version %s. Nothing to do.", upgrade.Name, versionToInstall)
			continue
		}
		// TODO: check "requirements-agent-release.txt" for min allowed version

		// Download the wheel
		wheelPath, err := downloadWheel(upgrade.Name, semverToPEP440(versionToInstall), upgrade.RootLayoutType)
		if err != nil {
			log.Errorf("Unexpected error while downloading wheel: %s", err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		// TODO: load the constraint file
		// rootDir, _ := executable.Folder()
		// constraintsPath := filepath.Join(rootDir, fmt.Sprintf("final_constraints-py%s.txt", "3"))
		// if _, err := os.Lstat(constraintsPath); err != nil {
		// 	log.Errorf("[RCM] Unexpected error while reading constraints file: %s", err)
		// 	applyStateCallback(configPath, state.ApplyStatus{
		// 		State: state.ApplyStateError,
		// 		Error: err.Error(),
		// 	})
		// 	continue
		// }

		// Install the wheel
		pipArgs := []string{
			"install",
			// "--constraint", constraintsPath,
			// We don't use pip to download wheels, so we don't need a cache
			"--no-cache-dir",
			// Specify to not use any index since we won't/shouldn't download anything with pip anyway
			"--no-index",
			// Do *not* install dependencies by default. This is partly to prevent
			// accidental installation / updates of third-party dependencies from PyPI.
			"--no-deps",
		}
		if err := pip(append(pipArgs, wheelPath)); err != nil {
			log.Errorf("error installing wheel %s: %v", wheelPath, err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		// Move configuration files
		if err := moveConfigurationFilesOf(upgrade.Name); err != nil {
			log.Errorf("error moving configuration files %s: %v", wheelPath, err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		// Mark it as ack
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}
}

func getCommandPython() (string, error) {
	rootDir, _ := executable.Folder()
	pyPath := filepath.Join(rootDir, "..", "..", "venv3", "bin", "python")
	// pyPath := filepath.Join(rootDir, filepath.Join("embedded", "bin", fmt.Sprintf("%s%s", "python", "3")))

	if _, err := os.Stat(pyPath); err != nil {
		if os.IsNotExist(err) {
			return pyPath, errors.New(fmt.Sprintf("unable to find python executable at %s", pyPath))
		}
	}

	return pyPath, nil
}

func semverToPEP440(version *semver.Version) string {
	pep440 := fmt.Sprintf("%d.%d.%d", version.Major(), version.Minor(), version.Patch())
	if version.Prerelease() == "" {
		return pep440
	}
	parts := strings.SplitN(string(version.Prerelease()), ".", 2)
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

func pep440ToSemver(pep440 string) (*semver.Version, error) {
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

func downloadWheel(integration, version, rootLayoutType string) (string, error) {
	// We use python 3 to invoke the downloader regardless of config
	pyPath, err := getCommandPython()
	if err != nil {
		return "", err
	}

	args := []string{
		"-m", downloaderModule,
		integration,
		"--version", version,
		// Can be core for integrations-core integrations, and extras for other integrations
		"--type", rootLayoutType,
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
	proxies := config.Datadog.GetProxies()
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
			fmt.Fprintf(os.Stderr, "%s\n", in.Text())
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

func pip(args []string) error {
	pythonPath, err := getCommandPython()
	if err != nil {
		return err
	}

	cmd := args[0]
	implicitFlags := args[1:]
	implicitFlags = append(implicitFlags, "--disable-pip-version-check")
	args = append([]string{"-mpip"}, cmd)

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
			log.Infof("%s\n", in.Text())
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
			log.Warnf("%s\n", in.Text())
		}
	}()

	err = pipCmd.Run()
	if err != nil {
		return fmt.Errorf("error running command: %v", err)
	}

	return nil
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

	pythonCmd := exec.Command(pythonPath, "-c", fmt.Sprintf(integrationVersionScriptPy3, integration))
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

	version, err := pep440ToSemver(outputStr)
	if err != nil {
		return nil, true, fmt.Errorf("error parsing version %s: %s", version, err)
	}

	return version, true, nil
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

func moveConfigurationFilesOf(integration string) error {
	confFolder := config.Datadog.GetString("confd_path")
	check := getIntegrationName(integration)
	confFileDest := filepath.Join(confFolder, fmt.Sprintf("%s.d", check))
	if err := os.MkdirAll(confFileDest, os.ModeDir|0755); err != nil {
		return err
	}

	rootDir, _ := executable.Folder()
	// TODO: use the embedded python
	relChecksPath := filepath.Join(rootDir, "..", "..", "venv3", "lib", "python3.8", "site-packages", "datadog_checks", check, "data")
	// relChecksPath, err := getRelChecksPath(cliParams)
	// confFileSrc := filepath.Join(rootDir, relChecksPath, check, "data")

	return moveConfigurationFiles(relChecksPath, confFileDest)
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
		log.Infof(
			"Successfully copied configuration file %s", filename,
		)
	}
	if errorMsg != "" {
		return fmt.Errorf(errorMsg)
	}
	return nil
}
