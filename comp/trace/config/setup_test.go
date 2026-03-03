// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConsumedLogFiles(t *testing.T) {
	t.Run("empty_confd_path", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": "",
		})
		result := loadConsumedLogFiles(cfg)
		assert.Nil(t, result)
	})

	t.Run("nonexistent_confd_path", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": "/nonexistent/path/that/does/not/exist",
		})
		result := loadConsumedLogFiles(cfg)
		assert.Nil(t, result)
	})

	t.Run("empty_confd_directory", func(t *testing.T) {
		dir := t.TempDir()
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Nil(t, result)
	})

	t.Run("exact_file_paths", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.yaml"), []byte(`
logs:
  - type: file
    path: /var/log/myapp.log
    service: myapp
    source: myapp
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Equal(t, []string{"/var/log/myapp.log"}, result)
	})

	t.Run("glob_pattern", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.yaml"), []byte(`
logs:
  - type: file
    path: /var/log/*.log
    service: myapp
    source: myapp
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Equal(t, []string{"/var/log/*.log"}, result)
	})

	t.Run("directory_path_cleaned", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.yaml"), []byte(`
logs:
  - type: file
    path: /var/log/myapp/
    service: myapp
    source: myapp
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Equal(t, []string{"/var/log/myapp"}, result)
	})

	t.Run("multiple_integrations", func(t *testing.T) {
		dir := t.TempDir()

		app1Dir := filepath.Join(dir, "app1.d")
		require.NoError(t, os.MkdirAll(app1Dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(app1Dir, "conf.yaml"), []byte(`
logs:
  - type: file
    path: /var/log/app1.log
    service: app1
`), 0644))

		app2Dir := filepath.Join(dir, "app2.d")
		require.NoError(t, os.MkdirAll(app2Dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(app2Dir, "conf.yaml"), []byte(`
logs:
  - type: file
    path: /var/log/app2/*.log
    service: app2
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		sort.Strings(result)
		assert.Equal(t, []string{"/var/log/app1.log", "/var/log/app2/*.log"}, result)
	})

	t.Run("skips_non_file_types", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.yaml"), []byte(`
logs:
  - type: tcp
    port: 10514
    service: myapp
  - type: file
    path: /var/log/myapp.log
    service: myapp
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Equal(t, []string{"/var/log/myapp.log"}, result)
	})

	t.Run("yml_extension", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.yml"), []byte(`
logs:
  - type: file
    path: /var/log/myapp.log
    service: myapp
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Equal(t, []string{"/var/log/myapp.log"}, result)
	})

	t.Run("custom_filename", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "custom_name.yaml"), []byte(`
logs:
  - type: file
    path: /var/log/custom.log
    service: myapp
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Equal(t, []string{"/var/log/custom.log"}, result)
	})

	t.Run("deduplicates_paths", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		// Two yaml files with the same log path
		content := []byte(`
logs:
  - type: file
    path: /var/log/myapp.log
    service: myapp
`)
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.yaml"), content, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "other.yaml"), content, 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Equal(t, []string{"/var/log/myapp.log"}, result)
	})

	t.Run("invalid_yaml_is_skipped", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.yaml"), []byte(`not: [valid: yaml: for: logs`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Nil(t, result)
	})

	t.Run("skips_plain_files_in_confd", func(t *testing.T) {
		dir := t.TempDir()
		// Create a regular file (not a directory) in confd_path
		require.NoError(t, os.WriteFile(filepath.Join(dir, "not_a_dir.yaml"), []byte(`
logs:
  - type: file
    path: /var/log/app.log
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Nil(t, result)
	})

	t.Run("skips_dirs_without_d_suffix", func(t *testing.T) {
		dir := t.TempDir()
		// Directory without .d suffix should be ignored
		plainDir := filepath.Join(dir, "myapp")
		require.NoError(t, os.MkdirAll(plainDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(plainDir, "conf.yaml"), []byte(`
logs:
  - type: file
    path: /var/log/app.log
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Nil(t, result)
	})

	t.Run("skips_non_yaml_files", func(t *testing.T) {
		dir := t.TempDir()
		integDir := filepath.Join(dir, "myapp.d")
		require.NoError(t, os.MkdirAll(integDir, 0755))
		// .json and .default files should be ignored
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.json"), []byte(`{"logs":[{"type":"file","path":"/var/log/app.log"}]}`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(integDir, "conf.yaml.default"), []byte(`
logs:
  - type: file
    path: /var/log/default.log
`), 0644))

		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"confd_path": dir,
		})
		result := loadConsumedLogFiles(cfg)
		assert.Nil(t, result)
	})
}
