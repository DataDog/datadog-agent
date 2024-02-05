// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package config

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/google/uuid"
)

const (
	defaultInstallType = "manual"
)

func applyOrCreateInstallSignature(c *config.AgentConfig) {
	s := &c.InstallSignature
	if s.Found {
		return
	}

	if c.ConfigPath == "" {
		return
	}

	// Try to read it from disk
	signaturePath := filepath.Join(filepath.Dir(c.ConfigPath), "install.json")
	err := readInstallSignatureFromDisk(signaturePath, s)
	if err == nil {
		s.Found = true
		return
	}

	// If not found, try to write it to disk
	err = generateNewInstallSignature(s)
	if err != nil {
		return
	}
	err = writeInstallSignatureToDisk(signaturePath, s)
	if err != nil {
		// If we can't persist it, don't use it
		return
	}
	s.Found = true
}

func readInstallSignatureFromDisk(path string, s *config.InstallSignatureConfig) (err error) {
	var file *os.File
	file, err = os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var fileContents []byte
	fileContents, err = io.ReadAll(file)
	if err != nil {
		return err
	}
	err = json.Unmarshal(fileContents, &s)
	if err != nil {
		return err
	}
	return nil
}

func generateNewInstallSignature(s *config.InstallSignatureConfig) (err error) {
	installID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	*s = config.InstallSignatureConfig{
		InstallID:   installID.String(),
		InstallType: defaultInstallType,
		InstallTime: time.Now().Unix(),
	}
	return nil
}

func writeInstallSignatureToDisk(path string, s *config.InstallSignatureConfig) error {
	contents, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0644)
}
