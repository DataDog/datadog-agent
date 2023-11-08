package api

import (
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"os"
)

type InstallSignature struct {
	InstallId   string `json:"install_id"`
	InstallType string `json:"install_type"`
}

const (
	defaultInstallType = "manual"
)

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
	installId, err := uuid.NewDCEGroup()
	if err != nil {
		return InstallSignature{}, err
	}
	return InstallSignature{
		InstallId:   installId.String(),
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
