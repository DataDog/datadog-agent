// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// TestGetScanKey asserts the scan key is suffixed with the source identifier only when a source,
// its config, and a non-empty identifier are all present — pinning each clause of that guard
// (a mutation of any && to || would either drop the suffix or dereference a nil source).
func TestGetScanKey(t *testing.T) {
	t.Run("container source with identifier", func(t *testing.T) {
		f := NewFile("/var/log/app.log", sources.NewLogSource("", &config.LogsConfig{Identifier: "abc123"}), false)
		assert.Equal(t, "/var/log/app.log/abc123", f.GetScanKey())
	})

	t.Run("source with empty identifier", func(t *testing.T) {
		f := NewFile("/var/log/app.log", sources.NewLogSource("", &config.LogsConfig{Identifier: ""}), false)
		assert.Equal(t, "/var/log/app.log", f.GetScanKey())
	})

	t.Run("source with nil config", func(t *testing.T) {
		f := NewFile("/var/log/app.log", sources.NewLogSource("", nil), false)
		assert.Equal(t, "/var/log/app.log", f.GetScanKey())
	})

	t.Run("nil source", func(t *testing.T) {
		f := &File{Path: "/var/log/app.log"}
		assert.Equal(t, "/var/log/app.log", f.GetScanKey())
	})
}
