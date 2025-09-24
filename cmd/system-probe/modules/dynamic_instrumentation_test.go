// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGoDIModuleOrder(t *testing.T) {
	allModules := All()
	assert.Less(t, slices.Index(allModules, EventMonitor), slices.Index(allModules, DynamicInstrumentation))
}
