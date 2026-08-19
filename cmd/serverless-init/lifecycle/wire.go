// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetupComponents holds the lifecycle dependencies that setup() threads
// into the lifecycle server and into mode.RunInit.
type SetupComponents struct {
	Port      int         // lifecycle hook server port; always set (defaults to DefaultPort)
	Handle    ChildHandle // always non-nil; noop in sidecar mode
	Child     *Child      // non-nil only in init-container mode (used by mode.RunInit)
	Forwarder *Forwarder  // non-nil only when env var is valid AND init-container mode
}

// setupInput holds the raw string values for setupComponents. Empty string means
// "unset / use default" for each field, matching env-var semantics.
type setupInput struct {
	userAppPort   string
	lifecyclePort string
	forwardMs     string
	readyMs       string
	validateMs    string
	sidecarMode   bool
}

// SetupFromEnv reads lifecycle configuration from environment variables and
// delegates to setupComponents.
func SetupFromEnv(sidecarMode bool) (SetupComponents, error) {
	return setupComponents(setupInput{
		userAppPort:   os.Getenv(UserAppPortEnvVar),
		lifecyclePort: os.Getenv(LifecyclePortEnvVar),
		forwardMs:     os.Getenv(ForwardTimeoutMsEnvVar),
		readyMs:       os.Getenv(ReadyTimeoutMsEnvVar),
		validateMs:    os.Getenv(ValidateTimeoutMsEnvVar),
		sidecarMode:   sidecarMode,
	})
}

// SetupFallback returns components with the default port and no forwarder. Used
// when SetupFromEnv fails (e.g. invalid user-app port) so the lifecycle server
// still starts and the platform can complete lifecycle handshakes.
func SetupFallback(sidecarMode bool) SetupComponents {
	// zero-value setupInput → all defaults; always succeeds
	c, err := setupComponents(setupInput{sidecarMode: sidecarMode})
	if err != nil {
		panic(fmt.Sprintf("BUG: SetupFallback failed with zero-value setupInput: %v", err))
	}
	return c
}

// setupComponents is the testable inner function.
func setupComponents(in setupInput) (SetupComponents, error) {
	lifecyclePort, err := parsePort(LifecyclePortEnvVar, in.lifecyclePort, DefaultPort)
	if err != nil {
		return SetupComponents{}, err
	}

	// The forwarder is never built in sidecar mode, so userAppPort and the
	// forward/ready/validate timeouts are irrelevant here. Returning before
	// parsing them means a stale or colliding value inherited from an
	// init-mode config (e.g. equal to the lifecycle port) only produces a
	// warning instead of failing setup and forcing a fallback that would
	// also discard unrelated, valid settings like a custom lifecycle port.
	if in.sidecarMode {
		if in.userAppPort != "" {
			log.Warnf("%s is set in sidecar mode; forwarder is disabled (init-container mode is required)", UserAppPortEnvVar)
		}
		return SetupComponents{Port: lifecyclePort, Handle: NewNoopChildHandle()}, nil
	}

	userAppPort, err := parsePort(UserAppPortEnvVar, in.userAppPort, 0, lifecyclePort)
	if err != nil {
		return SetupComponents{}, err
	}

	forwardTimeout, err := parseDurationMs(ForwardTimeoutMsEnvVar, in.forwardMs, defaultForwardTimeout)
	if err != nil {
		return SetupComponents{}, err
	}

	readyTimeout, err := parseDurationMs(ReadyTimeoutMsEnvVar, in.readyMs, defaultReadyTimeout)
	if err != nil {
		return SetupComponents{}, err
	}

	validateTimeout, err := parseDurationMs(ValidateTimeoutMsEnvVar, in.validateMs, defaultValidateTimeout)
	if err != nil {
		return SetupComponents{}, err
	}

	c := SetupComponents{Port: lifecyclePort}
	c.Child = NewChild()
	c.Handle = c.Child
	if userAppPort != 0 {
		c.Forwarder = NewForwarder(userAppPort, forwardTimeout, readyTimeout, validateTimeout)
	}
	return c, nil
}
