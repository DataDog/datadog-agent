// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installinfo offers helpers to interact with the 'install_info'/'install.json' files.
package installinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	execTimeout   = 30 * time.Second
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

	tool, toolVersion, installerVersion := getToolVersion(installType)

	info := map[string]map[string]string{
		"install_method": {
			"tool":              tool,
			"tool_version":      toolVersion,
			"installer_version": installerVersion,
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
		"install_type": installerVersion,
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

func getToolVersion(installType string) (tool string, toolVersion string, installerVersion string) {
	tool = toolInstaller
	toolVersion = version.AgentVersion
	installerVersion = fmt.Sprintf("%s_package", installType)
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		tool = "dpkg"
		toolVersion, err = getDpkgVersion()
		if err != nil {
			toolVersion = "unknown"
		}
		toolVersion = fmt.Sprintf("dpkg-%s", toolVersion)
	}
	if _, err := exec.LookPath("rpm"); err == nil {
		tool = "rpm"
		toolVersion, err = getRPMVersion()
		if err != nil {
			toolVersion = "unknown"
		}
		toolVersion = fmt.Sprintf("rpm-%s", toolVersion)
	}
	return
}

func getRPMVersion() (string, error) {
	cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
	defer cancelfunc()
	output, err := exec.CommandContext(cancelctx, "rpm", "-q", "-f", "/bin/rpm", "--queryformat", "%%{VERSION}").Output()
	return string(output), err
}

func getDpkgVersion() (string, error) {
	cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
	defer cancelfunc()
	cmd := exec.CommandContext(cancelctx, "dpkg-query", "--showformat=${Version}", "--show", "dpkg")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to get dpkg version: %s", err)
		return "", err
	}
	splitVersion := strings.Split(strings.TrimSpace(string(output)), ".")
	if len(splitVersion) < 3 {
		return "", fmt.Errorf("failed to parse dpkg version: %s", string(output))
	}
	return strings.Join(splitVersion[:3], "."), nil
}
