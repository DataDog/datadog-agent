// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"fmt"
	"strconv"
)

// UserAppPortEnvVar opts in to forwarding lifecycle hooks to the user app.
const UserAppPortEnvVar = "DD_SERVERLESS_MICROVM_USER_APP_PORT"

func parseUserAppPort(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: must be an integer (got %q)", UserAppPortEnvVar, raw)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s: must be in [1, 65535] (got %d)", UserAppPortEnvVar, port)
	}
	if port == DefaultPort {
		return 0, fmt.Errorf("%s: must not equal %d (collides with the lifecycle server)", UserAppPortEnvVar, DefaultPort)
	}
	return port, nil
}
