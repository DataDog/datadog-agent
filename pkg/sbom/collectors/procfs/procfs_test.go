// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && linux

package procfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

func TestScanRoot(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{
		"etc/os-release":       "ID=alpine\nVERSION_ID=3.20.0\n",
		"lib/apk/db/installed": "P:musl\nV:1.2.5-r0\nA:x86_64\no:musl\n\n",
	} {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
	}
	c := &Collector{
		trivyCollector: trivy.NewCollectorForCLI(),
		opts:           sbom.ScanOptions{Analyzers: []string{trivy.OSAnalyzers}},
	}

	t.Run("root", func(t *testing.T) {
		result := c.scanRoot(t.Context(), "container", root)
		require.NoError(t, result.Error)
		assert.Equal(t, "container", result.RequestID)
		assert.NotEmpty(t, result.Report.ToCycloneDX().GetComponents())
	})

	// The scan reads a missing root as an empty filesystem.
	t.Run("root gone", func(t *testing.T) {
		result := c.scanRoot(t.Context(), "container", filepath.Join(root, "gone"))
		assert.Equal(t, ErrNotFound, result.Error)
		assert.Nil(t, result.Report)
	})
}
