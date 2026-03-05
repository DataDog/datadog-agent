// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package ssi

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// iisInstrumentedRegistryKey is the registry key used to store the IIS instrumentation status.
	// It is set by the fleet installer when IIS instrumentation is enabled or disabled.
	// The agent reads this key to report the IIS instrumentation status without requiring admin permissions.
	iisInstrumentedRegistryKey   = `SOFTWARE\Datadog\APM SSI`
	iisInstrumentedRegistryValue = "IISInstrumented"
)

// SetIISInstrumentedMarker sets or clears the registry marker for IIS instrumentation.
// This should be called by the fleet installer after enabling or disabling IIS instrumentation.
func SetIISInstrumentedMarker(enabled bool) error {
	if enabled {
		k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, iisInstrumentedRegistryKey, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("could not create IIS instrumentation registry key: %w", err)
		}
		defer k.Close()
		return k.SetDWordValue(iisInstrumentedRegistryValue, 1)
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, iisInstrumentedRegistryKey, registry.SET_VALUE)
	if err != nil {
		// key doesn't exist, nothing to clear
		return nil
	}
	defer k.Close()
	err = k.DeleteValue(iisInstrumentedRegistryValue)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("could not delete IIS instrumentation registry value: %w", err)
	}
	return nil
}

// GetInstrumentationStatus returns the status of the APM auto-instrumentation on Windows.
func GetInstrumentationStatus() (status APMInstrumentationStatus, err error) {
	// IIS instrumentation: check registry marker set by the fleet installer.
	// Reading applicationHost.config requires admin permissions, so we use a registry
	// key set by the fleet installer (which runs with sufficient privileges) instead.
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, iisInstrumentedRegistryKey, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		val, _, err := k.GetIntegerValue(iisInstrumentedRegistryValue)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return status, fmt.Errorf("could not read IIS instrumentation registry value: %w", err)
		}
		status.IISInstrumented = val == 1
	}

	// Host instrumentation: check if the DDInjector kernel driver service is running
	running, svcErr := winutil.IsServiceRunning("DDInjector")
	if svcErr != nil && !errors.Is(svcErr, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return status, fmt.Errorf("could not check DDInjector service: %w", svcErr)
	}
	status.HostInstrumented = running

	return status, nil
}
