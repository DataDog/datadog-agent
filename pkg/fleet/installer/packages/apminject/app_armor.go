// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	appArmorAbstractionDir       = "/etc/apparmor.d/abstractions/"
	appArmorBaseProfile          = "/etc/apparmor.d/abstractions/base"
	appArmorDatadogDir           = appArmorAbstractionDir + "datadog.d/"
	appArmorInjectorProfilePath  = appArmorDatadogDir + "injector"
	appArmorBaseDIncludeIfExists = "include if exists <abstractions/datadog.d>"
	appArmorProfile              = `/opt/datadog-packages/** rix,
/proc/@{pid}/** rix,
/run/datadog/apm.socket rw,`
)

var (
	appArmorEnabledPath = "/sys/module/apparmor/parameters/enabled"
	appArmorProfileDir  = "/etc/apparmor.d"
	systemdIsRunning    = systemd.IsRunning
)

func findAndReplaceAllInFile(filename string, needle string, replaceWith string) error {
	haystack, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	haystack = []byte(strings.ReplaceAll(string(haystack), needle, replaceWith))

	if err = os.WriteFile(filename, haystack, 0); err != nil {
		return err
	}

	return nil
}

func unpatchBaseProfileWithDatadogInclude(filename string) error {

	// make sure base profile exists before we continue
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return findAndReplaceAllInFile(filename, "\n"+appArmorBaseDIncludeIfExists, "")
}

func patchBaseProfileWithDatadogInclude(filename string) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// we'll be going through the whole base profile looking for a sign indicating this version
	// supports the base.d extra profiles if it exists we'll return true
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), appArmorBaseDIncludeIfExists) {
			return nil
		}
	}

	if err = scanner.Err(); err != nil {
		return err
	}

	// add a new line
	if _, err = file.WriteString("\n" + appArmorBaseDIncludeIfExists); err != nil {
		return err
	}

	if err = file.Sync(); err != nil {
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

	// check if apparmor is running properly by executing aa-status
	// https://manpages.ubuntu.com/manpages/noble/man8/aa-status.8.html
	if err = telemetry.CommandContext(ctx, "aa-status").
		WithExpectedExitCodes(
			1, // apparmor not enabled/loaded
			2, // apparmor enabled but no policy loaded
			3, // apparmor control files unavailable under /sys/kernel/security/ (common in containers)
		).Run(); err != nil {
		// no-op is apparmor is not running properly
		return nil
	}

	// make sure base profile exists before we continue
	if _, err = os.Stat(appArmorBaseProfile); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	span, _ := telemetry.StartSpanFromContext(ctx, "setup_app_armor")
	defer func() { span.Finish(err) }()

	// first make sure base.d exists before we add it to the base profile
	// minimize the chance for a race
	if err = os.MkdirAll(appArmorDatadogDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", appArmorDatadogDir, err)
	}
	// unfortunately this isn't an atomic change. All files in that directory can be interpreted
	// and I did not implement finding a safe directory to write to in the same partition, to run an atomic move.
	// This shouldn't be a problem as we reload app armor right after writing the file.
	if err = os.WriteFile(appArmorInjectorProfilePath, []byte(appArmorProfile), 0644); err != nil {
		return err
	}

	if err = patchBaseProfileWithDatadogInclude(appArmorBaseProfile); err != nil {
		return fmt.Errorf("failed validate %s contains an include to base.d: %w", appArmorBaseProfile, err)
	}

	if err = reloadAppArmor(ctx); err != nil {
		if rollbackErr := os.Remove(appArmorInjectorProfilePath); rollbackErr != nil {
			log.Warnf("failed to remove apparmor profile: %v", rollbackErr)
		}
		return err
	}
	return nil
}

func removeAppArmor(ctx context.Context) (err error) {
	_, err = os.Stat(appArmorInjectorProfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_app_armor")
	defer span.Finish(err)

	// first unpatch and then delete the profile
	if err = unpatchBaseProfileWithDatadogInclude(appArmorBaseProfile); err != nil {
		return err
	}

	if err = os.Remove(appArmorInjectorProfilePath); err != nil {
		return err
	}
	_ = reloadAppArmor(ctx)
	return nil
}

func reloadAppArmor(ctx context.Context) error {
	if !isAppArmorRunning() {
		return nil
	}
	if running, err := systemdIsRunning(); err != nil {
		return err
	} else if running {
		masked, err := isSystemdUnitMasked(ctx, "apparmor")
		if err != nil {
			return err
		}
		if masked {
			log.Infof("Installer: apparmor.service is masked, reloading profiles with apparmor_parser")
			return reloadAppArmorWithParser(ctx)
		}
		return telemetry.CommandContext(ctx, "systemctl", "reload", "apparmor").Run()
	}
	return telemetry.CommandContext(ctx, "service", "apparmor", "reload").Run()
}

func reloadAppArmorWithParser(ctx context.Context) error {
	if _, err := exec.LookPath("apparmor_parser"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			log.Warnf("apparmor_parser not found, skipping AppArmor reload")
			return nil
		}
		return err
	}
	enforceProfiles, complainProfiles, err := appArmorProfiles()
	if err != nil {
		return err
	}
	if len(enforceProfiles) > 0 {
		if err := telemetry.CommandContext(ctx, "apparmor_parser", append([]string{"-r"}, enforceProfiles...)...).Run(); err != nil {
			return err
		}
	}
	if len(complainProfiles) > 0 {
		return telemetry.CommandContext(ctx, "apparmor_parser", append([]string{"-r", "-C"}, complainProfiles...)...).Run()
	}
	return nil
}

func appArmorProfiles() (enforceProfiles []string, complainProfiles []string, err error) {
	paths, err := filepath.Glob(filepath.Join(appArmorProfileDir, "*"))
	if err != nil {
		return nil, nil, err
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		name := filepath.Base(path)
		if pathExists(filepath.Join(appArmorProfileDir, "disable", name)) {
			continue
		}
		if pathExists(filepath.Join(appArmorProfileDir, "force-complain", name)) {
			complainProfiles = append(complainProfiles, path)
			continue
		}
		enforceProfiles = append(enforceProfiles, path)
	}
	return enforceProfiles, complainProfiles, nil
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isSystemdUnitMasked(ctx context.Context, unit string) (bool, error) {
	var stdout bytes.Buffer
	cmd := telemetry.CommandContext(ctx, "systemctl", "is-enabled", unit).
		WithExpectedExitCodes(
			1, // disabled, masked, static, indirect, generated, transient — https://man7.org/linux/man-pages/man1/systemctl.1.html
			4, // no such unit file — https://man7.org/linux/man-pages/man1/systemctl.1.html
		)
	cmd.Stdout = &stdout
	err := cmd.Run()
	if strings.TrimSpace(stdout.String()) == "masked" {
		return true, nil
	}
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return false, nil
}

func isAppArmorRunning() bool {
	data, err := os.ReadFile(appArmorEnabledPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "Y"
}
