// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package common

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (s *Setup) postInstallPackages() (err error) {
	s.addAgentToAdditionalGroups()

	return nil
}

func (s *Setup) addAgentToAdditionalGroups() {
	for _, group := range s.DdAgentAdditionalGroups {
		// Add dd-agent user to additional group for permission reason, in particular to enable reading log files not world readable
		if _, err := user.LookupGroup(group); err != nil {
			log.Infof("Skipping group %s as it does not exist", group)
			continue
		}
		_, err := ExecuteCommandWithTimeout(s, "usermod", "-aG", group, "dd-agent")
		if err != nil {
			s.Out.WriteString("Failed to add dd-agent to group" + group + ": " + err.Error())
			log.Warnf("failed to add dd-agent to group %s:  %v", group, err)
		}
	}
}

func copyInstallerSSI() error {
	destinationPath := "/opt/datadog-packages/run/datadog-installer-ssi"

	// Get the current executable path
	currentExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable: %w", err)
	}

	// Open the current executable file
	sourceFile, err := os.Open(currentExecutable)
	if err != nil {
		return fmt.Errorf("failed to open current executable: %w", err)
	}
	defer sourceFile.Close()

	// Create /usr/bin directory if it doesn't exist (unlikely)
	err = os.MkdirAll(filepath.Dir(destinationPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create installer directory: %w", err)
	}

	// Check if the destination file already exists and remove it if it does (we don't want to overwrite a symlink)
	if _, err := os.Stat(destinationPath); err == nil {
		if err := os.Remove(destinationPath); err != nil {
			return fmt.Errorf("failed to remove existing destination file: %w", err)
		}
	}

	// Create the destination file
	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close()

	// Copy the current executable to the destination file
	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy executable: %w", err)
	}

	// Set the permissions on the destination file to be executable
	err = destinationFile.Chmod(0755)
	if err != nil {
		return fmt.Errorf("failed to set permissions on destination file: %w", err)
	}

	return nil
}
