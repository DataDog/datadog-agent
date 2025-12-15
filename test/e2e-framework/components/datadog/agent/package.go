// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	tifos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// GetPackagePath retrieve the name of the package that should be installed.
// It will return the path to the package that should be installed for the given flavor and agent flavor.
// If the package is not found, it will return an error.
// If multiple packages are found, it will fail.
// Args:
//   - localPath: either a path to a folder or a path to a file
//     if a folder is provided, it should have a structure similar to what the agent CI exposes.
//     pkg/
//     | -- datadog-agent-<version>-<arch><fips?>.rpm
//     | -- datadog-agent-<version>-<arch><fips?>.deb
//     | -- suse/
//     |   | -- datadog-agent-<version>-<arch><fips?>.rpm
//   - flavor: the flavor of the host
//   - agentFlavor: the flavor of the agent
//   - arch: the architecture of the host
//   - pipelineID: the ID of the pipeline, if empty will be ignored, mainly used to avoid an issue with the Windows Runners, can be removed once CIEXE-143 is fixed
//
// Returns:
// - the path to the package that should be installed
// - an error if the package is not found or if there are multiple packages
func GetPackagePath(localPath string, flavor tifos.Flavor, agentFlavor string, arch tifos.Architecture, pipelineID string) (string, error) {
	var wantedExt string
	var subFolder string
	switch flavor {
	case tifos.AmazonLinux, tifos.CentOS, tifos.RedHat, tifos.AmazonLinuxECS, tifos.Fedora, tifos.RockyLinux:
		wantedExt = ".rpm"
	case tifos.Suse:
		wantedExt = ".rpm"
		subFolder = "suse"
	case tifos.Debian, tifos.Ubuntu:
		wantedExt = ".deb"
	case tifos.WindowsServer:
		wantedExt = ".msi"
	case tifos.MacosOS, tifos.Unknown:
		fallthrough
	default:
		return "", fmt.Errorf("unsupported flavor for local packages installation: %s", flavor)
	}

	pathInfo, err := os.Stat(localPath)
	if err != nil {
		return "", err
	}
	packagePath := localPath
	matches := []string{}
	if pathInfo.IsDir() {
		packagePath = path.Join(packagePath, subFolder, "pkg")

		// On Windows, if a dedicated pipeline-identified folder is available, use that
		if flavor == tifos.WindowsServer {
			packagePathWithPipelineID := path.Join(packagePath, "pipeline-"+pipelineID)
			if info, err := os.Stat(packagePathWithPipelineID); err == nil && info.IsDir() {
				packagePath = packagePathWithPipelineID
			}
		}

		entries, err := os.ReadDir(packagePath)
		if err != nil {
			return "", err
		}

		// First match all packages with the correct extension
		allPackagesPattern := `.*\.` + strings.TrimPrefix(wantedExt, ".") + `$`
		fipsPattern := `.*fips.*\.` + strings.TrimPrefix(wantedExt, ".") + `$`
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// Exclude -dbg_ packages
			if strings.Contains(entry.Name(), "-dbg_") || strings.Contains(entry.Name(), "-dbg-") {
				continue
			}

			// Exclude datadog-agent-upgrade-test package on Windows
			if flavor == tifos.WindowsServer && strings.Contains(entry.Name(), "upgrade-test") {
				continue
			}

			// Skip architecture check for Windows packages
			if flavor != tifos.WindowsServer {
				archStr := string(arch)
				if arch == tifos.AMD64Arch {
					if !strings.Contains(entry.Name(), "x86_64") && !strings.Contains(entry.Name(), "amd64") {
						continue
					}
				} else if !strings.Contains(entry.Name(), archStr) {
					continue
				}
			}

			// If we're on Windows, we need to check if the pipeline ID is in the package name, this is a workaround to avoid an issue with the Windows Runners
			// where other pipelines packages can pollute the artifacts
			if flavor == tifos.WindowsServer && pipelineID != "" {

				// If the package name contains "pipeline." and the pipeline ID is not in the name, skip it
				if strings.Contains(entry.Name(), "pipeline.") && !strings.Contains(entry.Name(), pipelineID) {
					continue
				}
			}

			// Arm64 is not supported for Windows
			if flavor == tifos.WindowsServer && arch == tifos.ARM64Arch {
				panic("arm64 is not supported for Windows")
			}

			// It would have been easier to use ^(?!.*fips).*\.deb$ with lookahead to match non-FIPS packages, but it is not supported by Go regex.
			// Instead we get all the packages and filter out the FIPS ones if we're looking for non-FIPS packages.
			if matched, _ := regexp.MatchString(allPackagesPattern, entry.Name()); matched {
				// If we're looking for FIPS packages, only include those
				if agentFlavor == agentparams.FIPSFlavor {
					if matched, _ := regexp.MatchString(fipsPattern, entry.Name()); matched {
						matches = append(matches, entry.Name())
					}
				} else {
					// If we're looking for non-FIPS packages, exclude FIPS ones
					if matched, _ := regexp.MatchString(fipsPattern, entry.Name()); !matched {
						matches = append(matches, entry.Name())
					}
				}
			}
		}

		if len(matches) == 0 {
			if agentFlavor == agentparams.FIPSFlavor {
				return "", fmt.Errorf("no FIPS package found in %s matching pattern %s", localPath, fipsPattern)
			}
			archInfo := ""
			if flavor != tifos.WindowsServer {
				if arch == tifos.AMD64Arch {
					archInfo = " with architecture x86_64 or amd64"
				} else {
					archInfo = fmt.Sprintf(" with architecture %s", arch)
				}
			}
			return "", fmt.Errorf("no package found in %s matching pattern %s without matching FIPS pattern %s%s", localPath, allPackagesPattern, fipsPattern, archInfo)
		}

		if len(matches) > 1 {
			return "", fmt.Errorf("found multiple packages to install:\n%s", strings.Join(matches, "\n"))
		}
		packagePath = path.Join(packagePath, matches[0])
	} else {
		if strings.HasSuffix(localPath, wantedExt) {
			return localPath, nil
		}
		return "", fmt.Errorf("local package %s does not have the expected extension %s", localPath, wantedExt)

	}
	return packagePath, nil
}
