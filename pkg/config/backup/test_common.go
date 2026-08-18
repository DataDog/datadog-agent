// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// writeConfigDir creates a temporary config directory with a datadog.yaml and
// returns a mock config whose ConfigFileUsed points at it, plus the directory.
func writeConfigDir(t *testing.T) (model.ReaderWriter, string) {
	t.Helper()
	dir := t.TempDir()
	confPath := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(confPath, []byte("api_key: 0000001\n"), 0o600))
	return configmock.NewFromFile(t, confPath), dir
}
