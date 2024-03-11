// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package selftests

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows/registry"
)

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(cfg *config.RuntimeSecurityConfig, probe *probe.Probe) (*SelfTester, error) {

	if !cfg.FIMEnabled {
		return nil, fmt.Errorf("FIM is disabled")
	}
	var (
		selfTests []SelfTest
		tmpDir    string
		tmpKey    string
	)

	if runtime.GOOS == "windows" {
		dir, err := CreateTargetDir()
		if err != nil {
			return nil, err
		}
		tmpDir = dir
		fileToCreate := "file.txt"

		keyName, err := createTempRegistryKey()
		if err != nil {
			return nil, err
		}
		tmpKey = keyName
		selfTests = []SelfTest{
			&WindowsCreateFileSelfTest{filename: fmt.Sprintf("%s/%s", dir, fileToCreate)},
			&WindowsSetRegistryKeyTest{keyName: keyName},
		}
	}

	s := &SelfTester{
		waitingForEvent: atomic.NewBool(cfg.EBPFLessEnabled),
		eventChan:       make(chan selfTestEvent, 10),
		selfTestRunning: make(chan time.Duration, 10),
		probe:           probe,
		selfTests:       selfTests,
		tmpDir:          tmpDir,
		tmpKey:          tmpKey,
		done:            make(chan bool),
		config:          cfg,
	}

	return s, nil
}

func createTempRegistryKey() (string, error) {

	keyName := fmt.Sprintf("datadog_agent_cws_self_test_temp_registry_key_%s", strconv.FormatInt(time.Now().UnixNano(), 10))

	baseKey, err := registry.OpenKey(registry.LOCAL_MACHINE, "Software\\Datadog", registry.WRITE)
	if err != nil {
		return "", fmt.Errorf("failed to open base key: %v", err)
	}
	defer baseKey.Close()

	// Create the temporary subkey under HKEY_CURRENT_USER\Software\Datadog
	tempKey, _, err := registry.CreateKey(baseKey, keyName, registry.WRITE)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary registry key: %v", err)
	}
	tempKey.Close()

	// Return the full path of the created temporary registry key
	return fmt.Sprintf("Software\\Datadog\\%s", keyName), nil
}

func deleteRegistryKey(path string) error {
	// Open base key
	baseKey, err := registry.OpenKey(registry.LOCAL_MACHINE, "Software\\Datadog", registry.WRITE)
	if err != nil {
		return fmt.Errorf("failed to open base key: %v", err)
	}
	defer baseKey.Close()

	// Delete the registry key
	if err := registry.DeleteKey(baseKey, fmt.Sprintf("HKLM:\\%s", path)); err != nil {
		return fmt.Errorf("failed to delete registry key: %v", err)
	}

	return nil
}

func Cleanup(tmpDir string, tmpKey string) error {
	if tmpDir != "" {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			log.Debugf("Error while deleting temporary file", err)
		}
		return err
	}

	if tmpKey != "" {
		err := deleteRegistryKey(tmpKey)
		return err
	}
	return nil

}
