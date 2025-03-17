// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package systemd offers an interface over systemd
package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// UnitsPath is the path where systemd unit files are stored
	UnitsPath = "/etc/systemd/system"
)

// StopUnit starts a systemd unit
func StopUnit(ctx context.Context, unit string, args ...string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "stop_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	args = append([]string{"stop", unit}, args...)
	err = exec.CommandContext(ctx, "systemctl", args...).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	if exitErr.ExitCode() == 5 {
		// exit code 5 means the unit is not loaded, we can continue
		return nil
	}
	return errors.New(string(exitErr.Stderr))
}

// StartUnit starts a systemd unit
func StartUnit(ctx context.Context, unit string, args ...string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "start_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	args = append([]string{"start", unit}, args...)
	err = exec.CommandContext(ctx, "systemctl", args...).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	// exit code 143 means the process was killed by a signal, most likely because we self stopped
	if exitErr.ExitCode() == 143 {
		return nil
	}
	return errors.New(string(exitErr.Stderr))
}

// EnableUnit enables a systemd unit
func EnableUnit(ctx context.Context, unit string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "enable_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	err = exec.CommandContext(ctx, "systemctl", "enable", unit).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	return errors.New(string(exitErr.Stderr))
}

// DisableUnit disables a systemd unit
func DisableUnit(ctx context.Context, unit string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "disable_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)

	enabledErr := exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", unit).Run()
	if enabledErr != nil {
		// unit is already disabled or doesn't exist, we can return fast
		return nil
	}

	err = exec.CommandContext(ctx, "systemctl", "disable", unit).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	if exitErr.ExitCode() == 5 {
		// exit code 5 means the unit is not loaded, we can continue
		return nil
	}
	return errors.New(string(exitErr.Stderr))
}

// WriteEmbeddedUnit writes a systemd unit from embedded resources
func WriteEmbeddedUnit(ctx context.Context, unit string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "write_embedded_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)

	var content []byte
	if strings.Contains(unit, "-exp.service") {
		content, err = generateExperimentUnit(ctx, unit)
	} else {
		content, err = embedded.FS.ReadFile(unit)
	}
	if err != nil {
		return fmt.Errorf("error getting unit %s: %w", unit, err)
	}
	err = os.MkdirAll(UnitsPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating systemd directory: %w", err)
	}
	unitPath := filepath.Join(UnitsPath, unit)
	return os.WriteFile(unitPath, content, 0644)
}

// RemoveUnit removes a systemd unit
func RemoveUnit(ctx context.Context, unit string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	err = os.Remove(path.Join(UnitsPath, unit))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// WriteUnitOverride writes a systemd unit override
func WriteUnitOverride(ctx context.Context, unit string, name string, content string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "write_unit_override")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	span.SetTag("name", name)
	err = os.MkdirAll(filepath.Join(UnitsPath, unit+".d"), 0755)
	if err != nil {
		return fmt.Errorf("error creating systemd directory: %w", err)
	}
	overridePath := filepath.Join(UnitsPath, unit+".d", fmt.Sprintf("%s.conf", name))
	return os.WriteFile(overridePath, []byte(content), 0644)
}

// Reload reloads the systemd daemon
func Reload(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "systemd_reload")
	defer func() { span.Finish(err) }()
	err = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	return errors.New(string(exitErr.Stderr))
}

// IsRunning checks if systemd is running using the documented way
// https://www.freedesktop.org/software/systemd/man/latest/sd_booted.html#Notes
func IsRunning() (running bool, err error) {
	_, err = os.Stat("/run/systemd/system")
	if os.IsNotExist(err) {
		log.Infof("Installer: systemd is not running, skip unit setup")
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

var (
	regexBefore      = regexp.MustCompile(`Before=(.*?)\n`)
	regexDescription = regexp.MustCompile(`Description=(.*)\n`)
	regexBindsTo     = regexp.MustCompile(`BindsTo=(.*)\n`)
	regexAlias       = regexp.MustCompile(`Alias=(.*)\n`)
	regexAfter       = regexp.MustCompile(`After=(.*)\n`)
	regexType        = regexp.MustCompile(`Type=(.*)\n`)
	regexWants       = regexp.MustCompile(`Wants=(.*)\n`)
	regexConflicts   = regexp.MustCompile(`Conflicts=(.*)\n`)
	regexRestart     = regexp.MustCompile(`Restart=(.*)\n`)
	regexStartLimits = regexp.MustCompile(`# Since systemd 229, should be in \[Unit\] but in order to support systemd <229,\n# it is also supported to have it here.\nStartLimitInterval=(.*?)\nStartLimitBurst=(.*?)\n`)
)

// generateExperimentUnit generates a systemd unit for an experiment
func generateExperimentUnit(ctx context.Context, unitName string) (unitExp []byte, err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "generate_experiment_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unitName)
	unitName = strings.ReplaceAll(unitName, "-exp.service", ".service")

	content, err := embedded.FS.ReadFile(unitName)
	if err != nil {
		return nil, fmt.Errorf("error reading embedded unit %s: %w", unitName, err)
	}

	if unitName == "datadog-agent.service" {
		return generateCoreAgentExperimentUnit(content)
	}

	if unitName == "datadog-installer.service" {
		// Cheat a bit here, we don't need to generate the unit file for the installer as it's deprecated
		content, err := embedded.FS.ReadFile("datadog-installer-exp.service")
		if err != nil {
			return nil, fmt.Errorf("error reading embedded unit %s: %w", "datadog-installer-exp.service", err)
		}
		return content, nil
	}

	return generateAgentSubprocessExperimentUnit(unitName, content)
}

func generateCoreAgentExperimentUnit(content []byte) ([]byte, error) {
	if submatches := regexAfter.FindSubmatch(content); len(submatches) > 1 {
		content = regexAfter.ReplaceAll(content, []byte("After="+string(submatches[1])+"\nOnFailure=datadog-agent.service\nJobTimeoutSec=3000\n"))
	} else {
		return nil, fmt.Errorf("could not find After in datadog-agent.service")
	}

	if submatches := regexWants.FindSubmatch(content); len(submatches) > 1 {
		content = regexWants.ReplaceAll(content, []byte("Wants="+strings.ReplaceAll(string(submatches[1]), ".service", "-exp.service")+"\n"))
	} else {
		return nil, fmt.Errorf("could not find Wants in datadog-agent.service")
	}

	content = regexType.ReplaceAll(content, []byte("Type=oneshot\n"))
	content = regexConflicts.ReplaceAll(content, []byte("Conflicts=datadog-agent.service\n"))
	content = regexBefore.ReplaceAll(content, []byte("Before=datadog-agent.service\n"))
	content = regexRestart.ReplaceAll(content, []byte(""))
	content = regexStartLimits.ReplaceAll(content, []byte("ExecStart=/bin/false\nExecStop=/bin/false\n"))

	if submatches := regexAlias.FindSubmatch(content); len(submatches) > 1 {
		content = regexAlias.ReplaceAll(content, []byte("Alias="+strings.ReplaceAll(string(submatches[1]), ".service", "-exp.service")+"\n"))
	}
	content = regexDescription.ReplaceAll(content, []byte("Description=$1 Experiment\n"))
	content = []byte(strings.ReplaceAll(string(content), "stable", "experiment"))

	return content, nil
}

func generateAgentSubprocessExperimentUnit(unitName string, content []byte) ([]byte, error) {
	content = regexBefore.ReplaceAll(content, []byte{})
	if submatches := regexBindsTo.FindSubmatch(content); len(submatches) > 1 {
		content = regexBindsTo.ReplaceAll(content, []byte("BindsTo="+strings.ReplaceAll(string(submatches[1]), ".service", "-exp.service")+"\n"))
	} else {
		return nil, fmt.Errorf("could not find BindsTo in %s", unitName)
	}

	if submatches := regexAfter.FindSubmatch(content); len(submatches) > 1 {
		afterNoAgent := strings.TrimSpace(strings.ReplaceAll(string(submatches[1]), "datadog-agent.service", ""))
		if afterNoAgent == "" {
			content = regexAfter.ReplaceAll(content, []byte{})
		} else {
			content = regexAfter.ReplaceAll(content, []byte("After="+afterNoAgent+"\n"))
		}
	} else {
		return nil, fmt.Errorf("could not find After in %s", unitName)
	}

	if submatches := regexAlias.FindSubmatch(content); len(submatches) > 1 {
		content = regexAlias.ReplaceAll(content, []byte("Alias="+strings.ReplaceAll(string(submatches[1]), ".service", "-exp.service")+"\n"))
	}
	content = regexDescription.ReplaceAll(content, []byte("Description=$1 Experiment\n"))
	content = []byte(strings.ReplaceAll(string(content), "stable", "experiment"))

	return content, nil
}
