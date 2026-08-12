// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd)

package configfilesdiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterEnvVars(t *testing.T) {
	selected := map[string]struct{}{
		"EMPTY":          {},
		"MISSING":        {},
		"REDIS_PASSWORD": {},
		"REDIS_PORT":     {},
		"WITH_EQUALS":    {},
	}

	env := filterEnvVars([]string{
		"REDIS_PORT=6379",
		"MALFORMED",
		"WITH_EQUALS=a=b=c",
		"EMPTY=",
		"REDIS_PORT=6380",
		"REDIS_PASSWORD=secret",
		"UNREQUESTED=value",
	}, func(name string) bool {
		_, ok := selected[name]
		return ok
	})

	assert.Equal(t, map[string]string{
		"EMPTY":       "",
		"REDIS_PORT":  "6380",
		"WITH_EQUALS": "a=b=c",
	}, env)
}
