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
	Port         int         // lifecycle hook server port; always set (defaults to DefaultPort)
	Handle       ChildHandle // always non-nil; noop in sidecar mode
	Child        *Child      // non-nil only in init-container mode (used by mode.RunInit)
	Forwarder    *Forwarder  // non-nil only when env var is valid AND init-container mode
	EnabledHooks HookToggles // per-hook forwarding opt-in; only meaningful when Forwarder != nil
}

// setupInput holds the raw string values for setupComponents. Empty string means
// "unset / use default" for each field, matching env-var semantics.
type setupInput struct {
	userAppPort     string
	lifecyclePort   string
	forwardMs       string
	readyMs         string
	validateMs      string
	enableReady     string
	enableValidate  string
	enableRun       string
	enableResume    string
	enableSuspend   string
	enableTerminate string
	sidecarMode     bool
}

// SetupFromEnv reads lifecycle configuration from environment variables and
// delegates to setupComponents.
func SetupFromEnv(sidecarMode bool) (SetupComponents, error) {
	return setupComponents(setupInput{
		userAppPort:     os.Getenv(UserAppPortEnvVar),
		lifecyclePort:   os.Getenv(LifecyclePortEnvVar),
		forwardMs:       os.Getenv(ForwardTimeoutMsEnvVar),
		readyMs:         os.Getenv(ReadyTimeoutMsEnvVar),
		validateMs:      os.Getenv(ValidateTimeoutMsEnvVar),
		enableReady:     os.Getenv(EnableReadyEnvVar),
		enableValidate:  os.Getenv(EnableValidateEnvVar),
		enableRun:       os.Getenv(EnableRunEnvVar),
		enableResume:    os.Getenv(EnableResumeEnvVar),
		enableSuspend:   os.Getenv(EnableSuspendEnvVar),
		enableTerminate: os.Getenv(EnableTerminateEnvVar),
		sidecarMode:     sidecarMode,
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

	enabled := HookToggles{
		Ready:     parseBoolFlag(EnableReadyEnvVar, in.enableReady, false),
		Validate:  parseBoolFlag(EnableValidateEnvVar, in.enableValidate, false),
		Run:       parseBoolFlag(EnableRunEnvVar, in.enableRun, false),
		Resume:    parseBoolFlag(EnableResumeEnvVar, in.enableResume, false),
		Suspend:   parseBoolFlag(EnableSuspendEnvVar, in.enableSuspend, false),
		Terminate: parseBoolFlag(EnableTerminateEnvVar, in.enableTerminate, false),
	}
	warnHookTogglesMismatch(userAppPort != 0, enabled)

	c := SetupComponents{Port: lifecyclePort, EnabledHooks: enabled}
	c.Child = NewChild()
	c.Handle = c.Child
	if userAppPort != 0 {
		c.Forwarder = NewForwarder(userAppPort, forwardTimeout, readyTimeout, validateTimeout)
	}
	return c, nil
}

// warnHookTogglesMismatch logs when the forwarder and the per-hook toggles
// disagree in a way the user likely did not intend:
//   - a forwarder is configured but no hook opted in (forwarding is fully
//     off, a change from the previous all-hooks-forward behavior), or
//   - a hook opted in but there is no forwarder to forward to.
func warnHookTogglesMismatch(hasForwarder bool, enabled HookToggles) {
	names := enabledHookNames(enabled)
	switch {
	case hasForwarder && len(names) == 0:
		log.Warnf("MicroVM lifecycle: %s is set but no hooks are enabled via DD_AWS_MICROVM_ENABLE_*; all hooks will use built-in handling instead of forwarding", UserAppPortEnvVar)
	case !hasForwarder && len(names) > 0:
		log.Warnf("MicroVM lifecycle: hook(s) %v enabled via DD_AWS_MICROVM_ENABLE_* but %s is not set; those hooks will use built-in handling", names, UserAppPortEnvVar)
	}
}

// enabledHookNames returns the lowercase hook names with their toggle set to true.
func enabledHookNames(t HookToggles) []string {
	var names []string
	if t.Ready {
		names = append(names, "ready")
	}
	if t.Validate {
		names = append(names, "validate")
	}
	if t.Run {
		names = append(names, "run")
	}
	if t.Resume {
		names = append(names, "resume")
	}
	if t.Suspend {
		names = append(names, "suspend")
	}
	if t.Terminate {
		names = append(names, "terminate")
	}
	return names
}
