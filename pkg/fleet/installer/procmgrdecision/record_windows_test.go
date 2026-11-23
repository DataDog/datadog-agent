// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procmgrdecision

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

func TestDefaultStoreWindows(t *testing.T) {
	s := DefaultStore()
	assert.Equal(t, filepath.Join(paths.DatadogInstallerData, "process_manager"), s.Path())
}
