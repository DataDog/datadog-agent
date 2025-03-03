// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package installer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

// platformPrepareExperiment runs extra steps needed for the experiment on a specific platform
func (i *installerImpl) platformPrepareExperiment(pkg *oci.DownloadedPackage, repository *repository.Repository) error {
	// if we are installing the agent, we need to check if repository is setup
	if pkg.Name == packageDatadogAgent {
		// check if the repository is setup
		pkgState, err := repository.GetState()
		if err != nil {
			return fmt.Errorf("could not get repository state: %w", err)
		}
		// if package is not setup, we need to setup the repository
		// this means it was installed with the MSI manually
		if !pkgState.HasStable() {
			// need to setup the repository
			// create new temp folder for the repository
			tmpDirStable, err := i.packages.MkdirTemp()
			if err != nil {
				return fmt.Errorf("could not create temporary directory: %w", err)
			}
			defer os.RemoveAll(tmpDirStable)

			// get current installed version
			msiPath, installedVersion, err := packages.GetCurrentAgentMSIProperties()
			if err != nil {
				return fmt.Errorf("could not get installed version: %w", err)
			}

			// create copy of the msiPath in the new temp folder
			err = copyFile(msiPath, filepath.Join(tmpDirStable, fmt.Sprintf("datadog-agent-%s-1-x86_64.msi", installedVersion)))
			if err != nil {
				return fmt.Errorf("could not copy msi file: %w", err)
			}

			err = i.packages.Create(pkg.Name, installedVersion, tmpDirStable)
			if err != nil {
				return fmt.Errorf("could not create repository: %w", err)
			}
		}

	}
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	// Open the source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("could not open source file: %w", err)
	}
	defer sourceFile.Close()

	// Create the destination file
	destinationFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("could not create destination file: %w", err)
	}
	defer destinationFile.Close()

	// Copy the contents from source to destination
	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("could not copy file: %w", err)
	}

	// Flush the destination file to ensure all data is written
	err = destinationFile.Sync()
	if err != nil {
		return fmt.Errorf("could not flush destination file: %w", err)
	}

	return nil
}
