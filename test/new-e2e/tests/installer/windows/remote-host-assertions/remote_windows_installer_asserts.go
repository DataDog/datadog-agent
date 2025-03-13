package assertions

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// RemoteWindowsInstallerAssertions provides assertions for the Datadog Installer on Windows.
type RemoteWindowsInstallerAssertions struct {
	*RemoteWindowsBinaryAssertions
}

func (d *RemoteWindowsInstallerAssertions) execute(cmd string, options ...client.ExecuteOption) (string, error) {
	output, err := d.remoteHost.Execute(fmt.Sprintf("& \"%s\" %s", d.binaryPath, cmd), options...)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// Status provides assertions on the status output of the Datadog Installer.
func (d *RemoteWindowsInstallerAssertions) Status() *RemoteWindowsInstallerStatusAssertions {
	output, err := d.execute("status")
	d.require.NoError(err)
	return &RemoteWindowsInstallerStatusAssertions{
		RemoteWindowsInstallerAssertions: d,
		status:                           parseStatusOutput(output),
	}
}

// RemoteWindowsInstallerStatusAssertions provides assertions on the status output of the Datadog Installer.
type RemoteWindowsInstallerStatusAssertions struct {
	*RemoteWindowsInstallerAssertions
	status installerStatus
}

// HasPackage verifies that a package is present in the status output.
func (d *RemoteWindowsInstallerStatusAssertions) HasPackage(name string) *RemoteWindowsInstallerPackageAssertions {
	d.suite.T().Helper()
	d.require.Contains(d.status.Packages, name)
	return &RemoteWindowsInstallerPackageAssertions{
		RemoteWindowsInstallerStatusAssertions: d,
		pkg:                                    d.status.Packages[name],
	}
}

// RemoteWindowsInstallerPackageAssertions provides assertions on a package in the status output of the Datadog Installer.
type RemoteWindowsInstallerPackageAssertions struct {
	*RemoteWindowsInstallerStatusAssertions
	pkg packageStatus
}

// WithStableVersionEqual verifies the stable version of a package matches what's expected.
func (d *RemoteWindowsInstallerPackageAssertions) WithStableVersionEqual(version string) *RemoteWindowsInstallerPackageAssertions {
	d.suite.T().Helper()
	d.require.Equal(version, d.pkg.StableVersion, "expected matching stable version for package %s", d.pkg.Name)
	return d
}

// WithExperimentVersionEqual verifies the experiment version of a package matches what's expected.
func (d *RemoteWindowsInstallerPackageAssertions) WithExperimentVersionEqual(version string) *RemoteWindowsInstallerPackageAssertions {
	d.suite.T().Helper()
	d.require.Equal(version, d.pkg.ExperimentVersion, "expected matching experiment version for package %s", d.pkg.Name)
	return d
}

// WithStableVersionMatchPredicate verifies the stable version of a package by using a predicate function.
func (d *RemoteWindowsInstallerPackageAssertions) WithStableVersionMatchPredicate(predicate func(version string)) *RemoteWindowsInstallerPackageAssertions {
	d.suite.T().Helper()
	predicate(d.pkg.StableVersion)
	return d
}

// WithExperimentVersionMatchPredicate verifies the experiment version of a package by using a predicate function.
func (d *RemoteWindowsInstallerPackageAssertions) WithExperimentVersionMatchPredicate(predicate func(version string)) *RemoteWindowsInstallerPackageAssertions {
	d.suite.T().Helper()
	predicate(d.pkg.ExperimentVersion)
	return d
}

type packageStatus struct {
	Name              string
	StableVersion     string
	ExperimentVersion string
}

type installerStatus struct {
	Version  string
	Packages map[string]packageStatus
}

// TODO:
// Linux tests use curl to hit the unix socket and get JSON output but we can't do the same
// for the named pipe on Windows. We should consider adding a JSON output option to the status command.
func parseStatusOutput(output string) installerStatus {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentPackage packageStatus

	var status installerStatus
	status.Packages = make(map[string]packageStatus)

	// Regular expressions for extracting relevant lines
	versionRegex := regexp.MustCompile(`Datadog Installer v(\S+)`)
	packageNameRegex := regexp.MustCompile(`^\s*([a-zA-Z0-9\-_]+)$`)
	stableVersionRegex := regexp.MustCompile(`\s*. stable:\s*(\S+)`)
	experimentVersionRegex := regexp.MustCompile(`\s*. experiment:\s*(\S+)`)

	for scanner.Scan() {
		line := scanner.Text()

		if match := versionRegex.FindStringSubmatch(line); match != nil {
			status.Version = match[1]
			continue
		}

		// Check for package name
		if match := packageNameRegex.FindStringSubmatch(line); match != nil {
			// If we already have a package, store it before starting a new one
			if currentPackage.Name != "" {
				status.Packages[currentPackage.Name] = currentPackage
			}
			currentPackage = packageStatus{Name: match[1]}
			continue
		}

		// Check for stable version
		if match := stableVersionRegex.FindStringSubmatch(line); match != nil {
			currentPackage.StableVersion = match[1]
			continue
		}

		// Check for experiment version
		if match := experimentVersionRegex.FindStringSubmatch(line); match != nil {
			if match[1] == "none" {
				// handle this case here instead of in tests.
				// the JSON seems to use an empty string, so it'll save us some work later.
				continue
			}
			currentPackage.ExperimentVersion = match[1]
			continue
		}
	}

	// Append the last parsed package
	if currentPackage.Name != "" {
		status.Packages[currentPackage.Name] = currentPackage
	}

	return status
}
