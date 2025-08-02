// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"golang.org/x/sys/windows/registry"
)

const (
	// APMRegistryKey is the registry key path for APM configuration
	APMRegistryKey = `SOFTWARE\Datadog\Datadog Installer\APM`
	// APMInstrumentationMethodKey is the registry value name for APM instrumentation method
	APMInstrumentationMethodKey = "InstrumentationMethod"
)

func setAPMInstrumentationMethod(method string) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, APMRegistryKey, registry.ALL_ACCESS)
	if err != nil {
		// If the key doesn't exist, create it
		k, _, err = registry.CreateKey(registry.LOCAL_MACHINE, APMRegistryKey, registry.ALL_ACCESS)
		if err != nil {
			return err
		}
	}
	defer k.Close()

	fmt.Printf("setting APM instrumentation method to: %s\n", method)
	return k.SetStringValue(APMInstrumentationMethodKey, method)
}

func unsetAPMInstrumentationMethod() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, APMRegistryKey, registry.ALL_ACCESS)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()

	return k.DeleteValue(APMInstrumentationMethodKey)
}

func getAPMInstrumentationMethod() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, APMRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return env.APMInstrumentationNotSet, nil
		}
		return "", err
	}
	defer k.Close()

	method, _, err := k.GetStringValue(APMInstrumentationMethodKey)
	if err != nil {
		if err == registry.ErrNotExist {
			return env.APMInstrumentationNotSet, nil
		}
		return "", err
	}
	return method, nil
}

// ValidateAPMInstrumentationMethod validates that the provided method is supported
func ValidateAPMInstrumentationMethod(method string) error {
	if method != env.APMInstrumentationEnabledIIS && method != env.APMInstrumentationEnabledDotnet {
		return fmt.Errorf("Unsupported instrumentation method: %s", method)
	}
	return nil
}

// InstrumentAPMInjector instruments the APM injector for IIS on Windows
func InstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_injector")
	defer func() { span.Finish(err) }()

	return updateInstrumentation(ctx, method, "stable", false)
}

// UninstrumentAPMInjector un-instruments the APM injector for IIS on Windows
func UninstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_injector")
	defer func() { span.Finish(err) }()

	var currentMethod string
	currentMethod, err = getAPMInstrumentationMethod()
	if err != nil {
		return err
	}

	if currentMethod == env.APMInstrumentationNotSet && method == env.APMInstrumentationNotSet {
		return fmt.Errorf("No instrumentation method to uninstrument")
	}

	if currentMethod != env.APMInstrumentationNotSet && method == env.APMInstrumentationNotSet {
		method = currentMethod
	}

	err = uninstrumentDotnetLibrary(ctx, method, "stable")
	if err != nil {
		return err
	}

	// If we un-instrumented another method than the one currently set, we do not want to change the configuration
	if currentMethod != method {
		return nil
	}
	return unsetAPMInstrumentationMethod()
}
