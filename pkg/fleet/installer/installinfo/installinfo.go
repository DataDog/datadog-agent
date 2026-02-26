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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v2"
)

var (
	installInfoFile string
	installSigFile  string
)

const (
	toolInstaller = "installer"
	execTimeout   = 5 * time.Second
)

func init() {
	// TODO(WINA-1429): The data dir should be configurable on Windows
	installInfoFile = filepath.Join(paths.DatadogDataDir, "install_info")
	installSigFile = filepath.Join(paths.DatadogDataDir, "install.json")
}

// WriteInstallInfo writes install info and signature files.
func WriteInstallInfo(ctx context.Context, installType string) error {
	return writeInstallInfo(ctx, installInfoFile, installSigFile, installType, time.Now(), uuid.New().String())
}

func writeInstallInfo(ctx context.Context, installInfoFile string, installSigFile string, installType string, time time.Time, uuid string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "write_install_info")
	defer func() {
		span.Finish(err)
	}()
	span.SetTag("install_type", installType)
	span.SetTag("install_time", time.Unix())
	span.SetTag("install_id", strings.ToLower(uuid))
	span.SetTag("install_info_file", installInfoFile)
	span.SetTag("install_sig_file", installSigFile)

	// Don't overwrite existing install info file.
	if _, err := os.Stat(installInfoFile); err == nil {
		return nil
	}

	tool, toolVersion, installerVersion := getToolVersion(ctx, installType)
	span.SetTag("tool", tool)
	span.SetTag("tool_version", toolVersion)
	span.SetTag("installer_version", installerVersion)

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

func getToolVersion(ctx context.Context, installType string) (tool string, toolVersion string, installerVersion string) {
	tool = toolInstaller
	toolVersion = version.AgentVersion
	installerVersion = installType + "_package"
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		tool = "dpkg"
		toolVersion, err = getDpkgVersion(ctx)
		if err != nil {
			toolVersion = "unknown"
		}
		toolVersion = "dpkg-" + toolVersion
	}
	if _, err := exec.LookPath("rpm"); err == nil {
		tool = "rpm"
		toolVersion, err = getRPMVersion(ctx)
		if err != nil {
			toolVersion = "unknown"
		}
		toolVersion = "rpm-" + toolVersion
	}
	return
}

func getRPMVersion(ctx context.Context) (version string, err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "get_rpm_version")
	defer func() {
		span.Finish(err)
	}()
	cancelctx, cancelfunc := context.WithTimeout(ctx, execTimeout)
	defer cancelfunc()
	output, err := telemetry.CommandContext(cancelctx, "rpm", "-q", "-f", "/bin/rpm", "--queryformat", "%%{VERSION}").Output()
	return string(output), err
}

func getDpkgVersion(ctx context.Context) (version string, err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "get_dpkg_version")
	defer func() {
		span.Finish(err)
	}()
	cancelctx, cancelfunc := context.WithTimeout(ctx, execTimeout)
	defer cancelfunc()
	cmd := telemetry.CommandContext(cancelctx, "dpkg-query", "--showformat=${Version}", "--show", "dpkg")
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
