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
	Handle    ChildHandle // always non-nil; noop in sidecar mode
	Child     *Child      // non-nil only in init-container mode (used by mode.RunInit)
	Forwarder *Forwarder  // non-nil only when env var is valid AND init-container mode
}

// SetupFromEnv reads the env var and delegates to setupComponents.
func SetupFromEnv(sidecarMode bool) (SetupComponents, error) {
	return setupComponents(os.Getenv(UserAppPortEnvVar), sidecarMode)
}

// SetupFallback returns components with no forwarder. Used when SetupFromEnv
// fails (e.g. invalid port value) so the lifecycle server still starts on
// port 9000 and the platform can complete lifecycle handshakes.
func SetupFallback(sidecarMode bool) SetupComponents {
	c, err := setupComponents("", sidecarMode)
	if err != nil {
		panic(fmt.Sprintf("BUG: SetupFallback failed with empty port: %v", err))
	}
	return c
}

// setupComponents is the testable inner function. rawPort uses the same
// "empty string = unset" semantics as the env var.
func setupComponents(rawPort string, sidecarMode bool) (SetupComponents, error) {
	port, err := parseUserAppPort(rawPort)
	if err != nil {
		return SetupComponents{}, err
	}

	c := SetupComponents{}
	if sidecarMode {
		c.Handle = NewNoopChildHandle()
		if port != 0 {
			log.Warnf("%s is set in sidecar mode; forwarder is disabled (init-container mode is required)", UserAppPortEnvVar)
		}
		return c, nil
	}

	c.Child = NewChild()
	c.Handle = c.Child
	if port != 0 {
		c.Forwarder = NewForwarder(port)
	}
	return c, nil
}
