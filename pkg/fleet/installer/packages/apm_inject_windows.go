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
	// APMInjectionMethodKey is the registry value name for APM injection method
	APMInjectionMethodKey = "InjectionMethod"
)

func setAPMInjectionMethod(method string) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, APMRegistryKey, registry.ALL_ACCESS)
	if err != nil {
		// If the key doesn't exist, create it
		k, _, err = registry.CreateKey(registry.LOCAL_MACHINE, APMRegistryKey, registry.ALL_ACCESS)
		if err != nil {
			return err
		}
	}
	defer k.Close()

	fmt.Printf("setting APM injection method to: %s\n", method)
	return k.SetStringValue(APMInjectionMethodKey, method)
}

func unsetAPMInjectionMethod() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, APMRegistryKey, registry.ALL_ACCESS)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()

	return k.DeleteValue(APMInjectionMethodKey)
}

// getAPMInjectionMethod gets the APM injection method from the registry
func getAPMInjectionMethod() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, APMRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return env.APMInstrumentationNotSet, nil
		}
		return "", err
	}
	defer k.Close()

	method, _, err := k.GetStringValue(APMInjectionMethodKey)
	if err != nil {
		if err == registry.ErrNotExist {
			return env.APMInstrumentationNotSet, nil
		}
		return "", err
	}
	return method, nil
}

func ValidateAPMInstrumentationMethod(method string) error {
	if method != env.APMInstrumentationEnabledIIS && method != env.APMInstrumentationEnabledDotnet {
		return fmt.Errorf("Unsupported injection method: %s", method)
	}
	return nil
}

// InstrumentAPMInjector instruments the APM injector for IIS on Windows
func InstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_injector")
	defer func() { span.Finish(err) }()

	err = ValidateAPMInstrumentationMethod(method)
	if err != nil {
		return err
	}

	var currentMethod string
	currentMethod, err = getAPMInjectionMethod()
	if err != nil {
		return err
	}
	if currentMethod != env.APMInstrumentationNotSet && currentMethod != method {
		err = UninstrumentAPMInjector(ctx, currentMethod)
		if err != nil {
			return err
		}
	}

	err = instrumentDotnetLibrary(ctx, method, "stable")
	if err != nil {
		return err
	}

	return setAPMInjectionMethod(method)
}

// UninstrumentAPMInjector un-instruments the APM injector for IIS on Windows
func UninstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_injector")
	defer func() { span.Finish(err) }()

	var currentMethod string
	currentMethod, err = getAPMInjectionMethod()
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
	return unsetAPMInjectionMethod()
}
