// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	appArmorAbstractionDir       = "/etc/apparmor.d/abstractions/"
	appArmorBaseProfile          = "/etc/apparmor.d/abstractions/base"
	appArmorConfigPath           = appArmorBaseProfile + ".d"
	appArmorBaseDIncludeIfExists = "include if exists <abstractions/base.d>"
	appArmorBaseDInclude         = "include <abstractions/base.d>"
	appArmorProfile              = `/opt/datadog-packages/** rix,
/proc/@{pid}/** rix,
/run/datadog/apm.socket rw,`
)

var datadogProfilePath = filepath.Join(appArmorConfigPath, "datadog")

// ValidateBaseProfileIncludesBaseD verfies that a profile contains an include path to base.d
func ValidateBaseProfileIncludesBaseD(filename string) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// we'll be going through the whole base profile looking for a sign indicating this version
	// supports the base.d extra profiles if it exists we'll return true
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, appArmorBaseDInclude) || strings.Contains(line, appArmorBaseDIncludeIfExists) {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if _, err = file.WriteString("\n" + appArmorBaseDInclude + "\n"); err != nil {
		return err
	}
	return nil
}

func setupAppArmor(ctx context.Context) (err error) {
	_, err = exec.LookPath("aa-status")
	if err != nil {
		// no-op if apparmor is not installed
		return nil
	}
	span, _ := telemetry.StartSpanFromContext(ctx, "setup_app_armor")
	defer func() { span.Finish(err) }()

	// first make sure base.d exists before we add it to the base profile
	// minimize the chance for a race
	if err = os.MkdirAll(appArmorConfigPath, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", appArmorConfigPath, err)
	}
	// unfortunately this isn't an atomic change. All files in that directory can be interpreted
	// and I did not implement finding a safe directory to write to in the same partition, to run an atomic move.
	// This shouldn't be a problem as we reload app armor right after writing the file.
	if err = os.WriteFile(datadogProfilePath, []byte(appArmorProfile), 0644); err != nil {
		return err
	}

	if err = ValidateBaseProfileIncludesBaseD(appArmorBaseProfile); err != nil {
		return fmt.Errorf("failed validate %s contains an include to base.d: %w", appArmorBaseProfile, err)
	}

	if err = reloadAppArmor(); err != nil {
		if rollbackErr := os.Remove(datadogProfilePath); rollbackErr != nil {
			log.Warnf("failed to remove apparmor profile: %v", rollbackErr)
		}
		return err
	}
	return nil
}

func removeAppArmor(ctx context.Context) (err error) {
	_, err = os.Stat(datadogProfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_app_armor")
	defer span.Finish(err)
	if err = os.Remove(datadogProfilePath); err != nil {
		return err
	}
	return reloadAppArmor()
}

func reloadAppArmor() error {
	if !isAppArmorRunning() {
		return nil
	}
	if running, err := isSystemdRunning(); err != nil {
		return err
	} else if running {
		return exec.Command("systemctl", "reload", "apparmor").Run()
	}
	return exec.Command("service", "apparmor", "reload").Run()
}

func isAppArmorRunning() bool {
	data, err := os.ReadFile("/sys/module/apparmor/parameters/enabled")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "Y"
}
