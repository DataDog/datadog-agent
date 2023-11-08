// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package api

import (
	"io"
	"os"

	"encoding/json"
	"github.com/google/uuid"
)

// InstallSignature contains the information on how the agent was installed
// and a unique identifier that distinguishes this agent from others.
type InstallSignature struct {
	InstallID   string `json:"install_id"`
	InstallType string `json:"install_type"`
}

const (
	defaultInstallType = "manual"
)

// GetInstallSignature returns the install signature for this agent.
// If one is not found on disk, a new one is generated and written to disk.
func GetInstallSignature() (InstallSignature, error) {
	installSignature, err := readInstallSignatureFromDisk()
	if err == nil {
		return installSignature, nil
	}
	// If we could not read an install signature from disk, generate a new one.
	installSignature, err = generateNewInstallSignature()
	if err != nil {
		return InstallSignature{}, err
	}
	err = installSignature.writeToDisk()
	if err != nil {
		return InstallSignature{}, err
	}
	return installSignature, nil
}

func getInstallSignatureFilename() string {
	// TODO: Need to use a different path depending on the OS
	return "/etc/datadog-agent/install.json"
}

func readInstallSignatureFromDisk() (installSignature InstallSignature, err error) {
	var file *os.File
	file, err = os.Open(getInstallSignatureFilename())
	if err != nil {
		return InstallSignature{}, err
	}
	defer file.Close()
	var fileContents []byte
	fileContents, err = io.ReadAll(file)
	if err != nil {
		return InstallSignature{}, err
	}
	err = json.Unmarshal(fileContents, &installSignature)
	if err != nil {
		return InstallSignature{}, err
	}
	return installSignature, nil
}

func generateNewInstallSignature() (InstallSignature, error) {
	installID, err := uuid.NewDCEGroup()
	if err != nil {
		return InstallSignature{}, err
	}
	return InstallSignature{
		InstallID:   installID.String(),
		InstallType: defaultInstallType,
	}, nil
}

func (i InstallSignature) writeToDisk() error {
	contents, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return os.WriteFile(getInstallSignatureFilename(), contents, 0644)
}
