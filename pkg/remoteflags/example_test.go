// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflags_test

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Define your feature flags as typed constants
const (
	FlagEnableNewAlgorithm remoteflags.FlagName = "enable_new_algorithm"
	FlagEnableDebugMode    remoteflags.FlagName = "enable_debug_mode"
)

// Example shows basic usage of the Remote Flags system
func Example_basicUsage() {
	// Create a client (in real usage, get this from the rcflags component)
	client := remoteflags.NewClient()

	// Subscribe to a feature flag with mandatory error handling
	err := client.Subscribe(
		FlagEnableNewAlgorithm,
		// onChange: Called when the flag value changes, returns nil on success
		func(value remoteflags.FlagValue) error {
			if value {
				fmt.Println("New algorithm enabled")
				// Apply the change and return error if it fails
				return enableNewAlgorithm()
			} else {
				fmt.Println("New algorithm disabled")
				// Apply the change and return error if it fails
				return disableNewAlgorithm()
			}
		},
		// onNoConfig: Called when flag is not present in config
		func() {
			log.Info("Flag not present in configuration")
		},
		// onApplyError: REQUIRED - Handles errors and ensures correct state
		func(err error, failedValue remoteflags.FlagValue) {
			log.Errorf("Error with flag (value: %v): %v", failedValue, err)
			// Ensure feature is in a safe state
			forceAlgorithmToSafeState()
		},
	)

	if err != nil {
		log.Errorf("Failed to subscribe: %v", err)
	}
}

// Dummy functions for the example
func enableNewAlgorithm() error  { return nil }
func disableNewAlgorithm() error { return nil }
func forceAlgorithmToSafeState() {}

// Example_withComponent demonstrates usage within an Fx component
func Example_withComponent() {
	// This example shows the typical pattern for using Remote Flags
	// in a Datadog Agent component

	type MyComponent struct {
		flagsClient    *remoteflags.Client
		newAlgoEnabled bool
	}

	// In your component constructor
	newComponent := func(flagsClient *remoteflags.Client) *MyComponent {
		c := &MyComponent{
			flagsClient: flagsClient,
		}
		return c
	}

	// Subscribe during component initialization
	start := func(c *MyComponent) error {
		return c.flagsClient.Subscribe(
			FlagEnableNewAlgorithm,
			func(value remoteflags.FlagValue) error {
				c.newAlgoEnabled = bool(value)
				log.Infof("Algorithm mode changed: %v", value)
				return nil // Successfully applied
			},
			func() {
				log.Info("Algorithm flag not in config")
			},
			func(err error, failedValue remoteflags.FlagValue) {
				log.Errorf("Flag apply error (value: %v): %v, forcing safe state", failedValue, err)
				c.newAlgoEnabled = false // Safe default
			},
		)
	}

	// Usage
	c := newComponent(remoteflags.NewClient())
	if err := start(c); err != nil {
		log.Errorf("Failed to start: %v", err)
	}
}

// Example_multipleFlags demonstrates subscribing to multiple flags
func Example_multipleFlags() {
	client := remoteflags.NewClient()

	flags := []struct {
		name     remoteflags.FlagName
		onChange remoteflags.FlagChangeCallback
	}{
		{
			name: FlagEnableNewAlgorithm,
			onChange: func(value remoteflags.FlagValue) error {
				fmt.Printf("Algorithm: %v\n", value)
				return nil
			},
		},
		{
			name: FlagEnableDebugMode,
			onChange: func(value remoteflags.FlagValue) error {
				fmt.Printf("Debug mode: %v\n", value)
				return nil
			},
		},
	}

	for _, flag := range flags {
		flagName := flag.name // Capture for closure
		err := client.Subscribe(
			flag.name,
			flag.onChange,
			func() {
				log.Infof("Flag %s not in config", flagName)
			},
			func(err error, failedValue remoteflags.FlagValue) {
				log.Errorf("Flag %s apply error (value: %v): %v", flagName, failedValue, err)
			},
		)
		if err != nil {
			log.Errorf("Failed to subscribe to %s: %v", flag.name, err)
		}
	}
}

// Example_statefulFeature demonstrates managing feature state
func Example_statefulFeature() {
	type FeatureManager struct {
		client       *remoteflags.Client
		featureState map[remoteflags.FlagName]bool
	}

	manager := &FeatureManager{
		client:       remoteflags.NewClient(),
		featureState: make(map[remoteflags.FlagName]bool),
	}

	// Subscribe and track state
	manager.client.Subscribe(
		FlagEnableNewAlgorithm,
		func(value remoteflags.FlagValue) error {
			manager.featureState[FlagEnableNewAlgorithm] = bool(value)
			fmt.Printf("Feature state updated: %v\n", value)
			return nil
		},
		func() {
			log.Info("Feature flag not in config")
		},
		func(err error, failedValue remoteflags.FlagValue) {
			log.Errorf("Apply error (value: %v): %v", failedValue, err)
			manager.featureState[FlagEnableNewAlgorithm] = false
		},
	)

	// Check current state
	if enabled, exists := manager.client.GetCurrentValue(FlagEnableNewAlgorithm); exists {
		fmt.Printf("Current state: %v\n", enabled)
	} else {
		fmt.Println("State not yet known")
	}
}

// Example_errorHandlingStrategies demonstrates different error handling approaches
func Example_errorHandlingStrategies() {
	client := remoteflags.NewClient()

	// Strategy 1: Conservative - disable on error for experimental features
	client.Subscribe(
		"experimental_feature",
		func(value remoteflags.FlagValue) error {
			// Apply experimental feature
			return applyExperimentalFeature(bool(value))
		},
		func() {
			log.Info("Experimental feature flag not in config")
		},
		func(err error, failedValue remoteflags.FlagValue) {
			log.Errorf("Experimental feature apply error (value: %v): %v", failedValue, err)
			// Conservative: ensure experimental feature is disabled
			disableExperimentalFeature()
		},
	)

	// Strategy 2: Production-aligned - maintain stable state on error
	client.Subscribe(
		"production_optimization",
		func(value remoteflags.FlagValue) error {
			return applyProductionOptimization(bool(value))
		},
		func() {
			log.Info("Production optimization flag not in config")
		},
		func(err error, failedValue remoteflags.FlagValue) {
			log.Errorf("Production optimization apply error (value: %v): %v", failedValue, err)
			// Maintain current stable production behavior
			ensureStableProductionState()
		},
	)

	// Strategy 3: Feature-specific logic - logging should default to off
	client.Subscribe(
		"enhanced_logging",
		func(value remoteflags.FlagValue) error {
			return applyEnhancedLogging(bool(value))
		},
		func() {
			log.Info("Enhanced logging flag not in config")
		},
		func(err error, failedValue remoteflags.FlagValue) {
			log.Infof("Enhanced logging apply error (value: %v): %v", failedValue, err)
			// Enhanced logging should be off unless explicitly enabled
			disableEnhancedLogging()
		},
	)
}

// Dummy functions for examples
func applyExperimentalFeature(enabled bool) error    { return nil }
func disableExperimentalFeature()                    {}
func applyProductionOptimization(enabled bool) error { return nil }
func ensureStableProductionState()                   {}
func applyEnhancedLogging(enabled bool) error        { return nil }
func disableEnhancedLogging()                        {}
