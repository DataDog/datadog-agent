// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installinfo offers helpers to interact with the 'install_info'/'install.json' files.
package installinfo

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/google/uuid"
	"gopkg.in/yaml.v2"
)

const (
	installInfoFile = "/etc/datadog-agent/install_info"
	installSigFile  = "/etc/datadog-agent/install.json"

	toolInstaller = "installer"
)

// WriteInstallInfo writes install info and signature files.
func WriteInstallInfo(installType string) error {
	return writeInstallInfo(installInfoFile, installSigFile, installType, time.Now(), uuid.New().String())
}

func writeInstallInfo(installInfoFile string, installSigFile string, installType string, time time.Time, uuid string) error {
	// Don't overwrite existing install info file.
	if _, err := os.Stat(installInfoFile); err == nil {
		return nil
	}

	info := map[string]map[string]string{
		"install_method": {
			"tool":              toolInstaller,
			"tool_version":      version.AgentVersion,
			"installer_version": installType,
		},
	}
	yamlData, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal install info: %v", err)
	}
	if err := os.WriteFile(installInfoFile, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write install info file: %v", err)
	}

	sig := map[string]string{
		"install_id":   strings.ToLower(uuid),
		"install_type": installType,
		"install_time": strconv.FormatInt(time.Unix(), 10),
	}
	jsonData, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("failed to marshal install signature: %v", err)
	}
	if err := os.WriteFile(installSigFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write install signature file: %v", err)
	}
	return nil
}

// RemoveInstallInfo removes both install info and signature files.
func RemoveInstallInfo() {
	for _, file := range []string{installInfoFile, installSigFile} {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			log.Warnf("Failed to remove %s: %v", file, err)
		}
	}
}
