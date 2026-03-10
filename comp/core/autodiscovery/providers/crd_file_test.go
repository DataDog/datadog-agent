// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acTelemetry "github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

func newCRDTelemetryStore(t *testing.T) *acTelemetry.Store {
	tel := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	return acTelemetry.NewStore(tel)
}

// TestCRDCollectEmptyDirectory verifies that an empty directory returns an empty
// slice without error — this is normal startup state when no CRD checks are configured.
func TestCRDCollectEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	provider := NewCRDFileConfigProvider(dir, DefaultCRDNameExtractor, newCRDTelemetryStore(t))
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
	assert.Empty(t, provider.Errors)
}

// TestCRDCollectNonExistentDirectory verifies that a missing directory returns an
// empty slice. A missing directory is unexpected (the Operator/Helm always mounts
// the ConfigMap), so a warning is logged, but no error is returned to the caller.
func TestCRDCollectNonExistentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	provider := NewCRDFileConfigProvider(dir, DefaultCRDNameExtractor, newCRDTelemetryStore(t))
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}

// TestCRDCollectValidFileWithADIdentifiers verifies that a YAML file whose name
// follows the "<NAMESPACE_NAME_CHECKNAME>" convention is parsed correctly, that
// the check name comes from the filename (not the YAML), and that ADIdentifiers
// are populated, making the config a template (IsTemplate() == true).
func TestCRDCollectValidFileWithADIdentifiers(t *testing.T) {
	dir := t.TempDir()

	content := `ad_identifiers:
  - redis

init_config:

instances:
  - host: localhost
    port: 6379
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mynamespace_mypod_redis.yaml"), []byte(content), os.FileMode(0644)))

	provider := NewCRDFileConfigProvider(dir, DefaultCRDNameExtractor, newCRDTelemetryStore(t))
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0]
	assert.Equal(t, "redis", cfg.Name)
	assert.Equal(t, []string{"redis"}, cfg.ADIdentifiers)
	assert.True(t, cfg.IsTemplate(), "config with ad_identifiers should be a template")
	assert.Empty(t, provider.Errors)
}

// TestCRDCollectValidFileWithoutADIdentifiers verifies that a valid YAML file
// without ad_identifiers is collected and scheduled immediately (non-template).
func TestCRDCollectValidFileWithoutADIdentifiers(t *testing.T) {
	dir := t.TempDir()

	content := `init_config:

instances:
  - host: localhost
    port: 5432
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "default_dbpod_postgres.yaml"), []byte(content), os.FileMode(0644)))

	provider := NewCRDFileConfigProvider(dir, DefaultCRDNameExtractor, newCRDTelemetryStore(t))
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0]
	assert.Equal(t, "postgres", cfg.Name)
	assert.Empty(t, cfg.ADIdentifiers)
	assert.False(t, cfg.IsTemplate(), "config without ad_identifiers should not be a template")
	assert.Empty(t, provider.Errors)
}

// TestCRDCollectFilenameNotMatchingConvention verifies that a file whose name
// does not match the extractor convention is skipped and recorded in Errors.
func TestCRDCollectFilenameNotMatchingConvention(t *testing.T) {
	dir := t.TempDir()

	// Name has no '_', so DefaultCRDNameExtractor will return an error.
	content := `init_config:

instances:
  - foo: bar
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "badname.yaml"), []byte(content), os.FileMode(0644)))

	provider := NewCRDFileConfigProvider(dir, DefaultCRDNameExtractor, newCRDTelemetryStore(t))
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
	assert.Contains(t, provider.Errors, "badname")
}

// TestCRDCollectNonYAMLFileIgnored verifies that files with non-YAML extensions
// are silently skipped.
func TestCRDCollectNonYAMLFileIgnored(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "redis-mypod-mynamespace.txt"), []byte("some: content"), os.FileMode(0644)))

	provider := NewCRDFileConfigProvider(dir, DefaultCRDNameExtractor, newCRDTelemetryStore(t))
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
	assert.Empty(t, provider.Errors)
}

// TestCRDCollectSubdirectoriesIgnored verifies that subdirectories are silently
// skipped (flat-only layout).
func TestCRDCollectSubdirectoriesIgnored(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "redis-mypod-mynamespace.d")
	require.NoError(t, os.MkdirAll(subdir, os.FileMode(0755)))

	content := `init_config:

instances:
  - host: localhost
`
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "conf.yaml"), []byte(content), os.FileMode(0644)))

	provider := NewCRDFileConfigProvider(dir, DefaultCRDNameExtractor, newCRDTelemetryStore(t))
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}

// TestCRDCollectCustomNameExtractor verifies that injecting a custom
// NameExtractorFunc overrides the default extraction logic.
func TestCRDCollectCustomNameExtractor(t *testing.T) {
	dir := t.TempDir()

	content := `init_config:

instances:
  - host: localhost
    port: 9200
`
	// File name uses a custom '<CHECKNAME>-<TARGET>' convention (check name is the first segment).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "elasticsearch-node1-production.yaml"), []byte(content), os.FileMode(0644)))

	// Custom extractor: returns the portion before the first '-'.
	customExtractor := func(filenameWithoutExt string) (string, error) {
		idx := strings.Index(filenameWithoutExt, "-")
		if idx <= 0 {
			return "", fmt.Errorf("filename %q does not match custom convention", filenameWithoutExt)
		}
		return filenameWithoutExt[:idx], nil
	}

	provider := NewCRDFileConfigProvider(dir, customExtractor, newCRDTelemetryStore(t))
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, "elasticsearch", configs[0].Name)
}
