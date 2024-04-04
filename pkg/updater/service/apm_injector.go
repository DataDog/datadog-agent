// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"os"
	"path"
	"regexp"
)

var preloadBlockRegex = regexp.MustCompile(`^\/.+\/launcher\.preload\.so$`)

// SetupAPMInjector sets up the injector at bootstrap
func SetupAPMInjector() error {
	injectorPath := "/opt/datadog-packages/datadog-apm-inject/stable"
	return setupInjector(injectorPath)
}

// StartAPMInjectorExperiment sets up an APM injector experiment
func StartAPMInjectorExperiment() error {
	injectorPath := "/opt/datadog-packages/datadog-apm-inject/experiment"
	return setupInjector(injectorPath)
}

// StopAPMInjectorExperiment stops an APM injector experiment and reset to stable
func StopAPMInjectorExperiment() error {
	injectorPath := "/opt/datadog-packages/datadog-apm-inject/stable"
	return setupInjector(injectorPath)
}

func setupInjector(basePath string) error {
	// Set up owners & permissions for the run directory
	err := os.Chmod(path.Join(basePath, "run"), 0777)
	if err != nil {
		return err
	}

	// Add preload options on /etc/ld.so.preload, overriding existing ones
	// This loads the whole file in memory but it's fine, it should only be
	// a few lines long at most
	ldSoPreloadPath := "/etc/ld.so.preload"
	ldSoPreload, err := os.ReadFile(ldSoPreloadPath)
	if err != nil {
		return err
	}

	launcherPreloadPath := path.Join(basePath, "inject", "launcher.preload.so")
	launcherPreload, err := os.ReadFile(launcherPreloadPath)
	if err != nil {
		return err
	}

	// Replace or add the preload block
	if preloadBlockRegex.Match(ldSoPreload) {
		ldSoPreload = preloadBlockRegex.ReplaceAll(ldSoPreload, launcherPreload)
	} else {
		if ldSoPreload[len(ldSoPreload)-1] != '\n' {
			ldSoPreload = append(ldSoPreload, '\n')
		}
		ldSoPreload = append(ldSoPreload, launcherPreload...)
	}

	err = os.WriteFile(ldSoPreloadPath, ldSoPreload, 0644)
	if err != nil {
		return err
	}
	return nil
}
