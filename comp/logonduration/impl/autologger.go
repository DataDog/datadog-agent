// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package logondurationimpl implements the logon duration component
package logondurationimpl

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/tekert/goetw/etw"

	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	etlFileName           = "logon_duration.etl"
	autologgerSessionName = "Datadog Logon Duration"
	autologgerBasePath    = `SYSTEM\CurrentControlSet\Control\WMI\Autologger`
)

// stopAutologger stops the running ETW trace session
func stopAutologger(sessionName string) error {
	if err := etw.StopSession(sessionName); err != nil {
		return fmt.Errorf("stopping session '%s': %w", sessionName, err)
	}
	return nil
}

func getETLPath() (string, error) {
	pd, err := winutil.GetProgramDataDir()
	if err != nil {
		return "", fmt.Errorf("getting program data directory: %w", err)
	}
	fp := filepath.Join(pd, "logonduration", etlFileName)
	return fp, nil
}

// toggleAutologger enables or disables the AutoLogger by setting the Start registry value.
//   - enable=true:  Start=1 (trace will run on next boot)
//   - enable=false: Start=0 (trace will not run on next boot)
func toggleAutologger(sessionName string, enable bool) error {
	sessionPath := autologgerBasePath + `\` + sessionName

	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE, sessionPath, registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("opening autologger key '%s': %w (does the autologger exist? run 'create' first)", sessionName, err)
	}
	defer key.Close()

	var startValue uint32
	if enable {
		startValue = 1
	} else {
		startValue = 0
	}

	if err := key.SetDWordValue("Start", startValue); err != nil {
		return fmt.Errorf("setting Start value: %w", err)
	}

	return nil
}

func checkAutologgerExists(sessionName string) (bool, error) {
	sessionPath := autologgerBasePath + `\` + sessionName
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE, sessionPath, registry.QUERY_VALUE,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("opening autologger key '%s': %w", sessionName, err)
	}
	defer key.Close()
	return true, nil
}
