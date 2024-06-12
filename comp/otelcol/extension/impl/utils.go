// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package impl defines the OpenTelemetry Extension implementation.
package impl

import (
	"os"
	"strings"
)

func getEnvironmentAsMap() map[string]string {
	env := map[string]string{}
	for _, keyVal := range os.Environ() {
		split := strings.SplitN(keyVal, "=", 2)
		key, val := split[0], split[1]
		env[key] = val
	}

	return env
}
