// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/startup"
)

func TestConfigReaderStartupTracing(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("autoconf_config_files_num_workers", 4)
	cfg.SetInTest("ignore_autoconf", []string{"bar"})
	dir := t.TempDir()
	valid := "instances:\n  - password: never-export-this\n"
	metric := "jmx_metrics:\n  - include: {}\n"
	files := map[string]string{
		"foo.yaml":                 valid,
		"foo.yaml.default":         valid,
		"baz.yaml.default":         valid,
		"metrics.yaml":             metric, // Parsed but not collected from the root.
		"empty.yaml":               "",
		"invalid.yaml":             "instances: [",
		"ignored.txt":              "not a config",
		"bar.d/conf.yaml":          valid,
		"bar.d/conf.yaml.default":  valid,
		"bar.d/metrics.yaml":       metric,
		"bar.d/auto_conf.yaml":     valid, // Explicitly ignored.
		"bar.d/nested/hidden.yaml": valid, // Below the supported nesting depth.
		"skipdir/hidden.yaml":      valid, // Not an integration directory.
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
	}
	paths := []string{dir, t.TempDir(), filepath.Join(dir, "absent")}
	reference := &configFilesReader{paths: paths}
	wantConfigs, wantFormats, wantErrors := reference.read(GetAll)

	reader = nil
	doOnce = sync.Once{}
	t.Cleanup(func() {
		reader = nil
		doOnce = sync.Once{}
	})
	recorder := startup.NewRecorder(true)
	recorder.BeginHook(42)
	phase := recorder.Start("autodiscovery.config_files.initialize", "file")
	InitConfigFilesReaderWithTracing(paths, phase)
	phase.Finish(nil)
	gotConfigs, gotErrors, err := ReadConfigFiles(GetAll)
	require.NoError(t, err)
	require.Equal(t, wantConfigs, gotConfigs)
	require.Equal(t, wantErrors, gotErrors)
	require.Equal(t, wantFormats, ReadConfigFormats())

	// A second initializer must not scan again, and cache refreshes must not
	// retain the startup phase even while the surrounding hook is still active.
	reused := recorder.Start("reused", "file")
	InitConfigFilesReaderWithTracing(paths, reused)
	reused.Finish(nil)
	reader.cache.Flush()
	_, _, err = ReadConfigFiles(GetAll)
	require.NoError(t, err)
	recorder.EndHook()
	events, dropped := recorder.Drain()
	require.Zero(t, dropped)
	require.Len(t, events, 11) // Initializer + 3/3/1 path phases + defaults + cache + reuse.
	root := events[0]
	require.Equal(t, float64(1), root.Metrics["config_files.initial_scan"])
	require.Equal(t, float64(3), root.Metrics["config_files.search_paths"])
	require.Equal(t, float64(9), root.Metrics["config_files.files_attempted"])
	require.Equal(t, float64(9), root.Metrics["config_files.files_read"])
	require.Equal(t, float64(1), root.Metrics["config_files.empty_files"])
	require.Equal(t, float64(1), root.Metrics["config_files.config_errors"])
	require.Equal(t, float64(7), root.Metrics["config_files.configs_parsed"])
	require.Equal(t, float64(4), root.Metrics["config_files.configs_loaded"])
	require.Equal(t, float64(6), root.Metrics["config_files.config_formats"])
	require.Equal(t, float64(1), root.Metrics["config_files.nested_directories_read"])
	require.Zero(t, root.Metrics["config_files.read_errors"])
	var wantBytes int
	for _, name := range []string{"foo.yaml", "foo.yaml.default", "baz.yaml.default", "metrics.yaml", "empty.yaml", "invalid.yaml", "bar.d/conf.yaml", "bar.d/conf.yaml.default", "bar.d/metrics.yaml"} {
		wantBytes += len(files[name])
	}
	require.Equal(t, float64(wantBytes), root.Metrics["config_files.bytes_read"])

	for _, event := range events[1 : len(events)-1] {
		require.Equal(t, root.SpanID, event.ParentID)
		require.Equal(t, "file", event.Resource)
		require.False(t, event.Incomplete)
		require.False(t, event.Failed, "an absent optional directory is not an error")
		if event.Name == "autodiscovery.config_files.read_parse" {
			require.Equal(t, float64(4), event.Metrics["config_files.workers_configured"])
			if event.Metrics["config_files.path_index"] == 0 {
				require.Equal(t, float64(4), event.Metrics["config_files.workers_used"])
				require.Equal(t, root.Metrics["config_files.files_read"], event.Metrics["config_files.files_read"])
			} else {
				require.Zero(t, event.Metrics["config_files.workers_used"])
			}
		}
		if event.Name == "autodiscovery.config_files.merge_defaults" {
			require.Equal(t, float64(3), event.Metrics["config_files.defaults_seen"])
			require.Equal(t, float64(1), event.Metrics["config_files.defaults_added"])
		}
	}
	require.Equal(t, "reused", events[len(events)-1].Name)
	require.Zero(t, events[len(events)-1].Metrics["config_files.initial_scan"])
	payload, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(payload), dir)
	require.NotContains(t, string(payload), "never-export-this")
	require.NotContains(t, string(payload), "foo.yaml")
}

func TestConfigReaderWorkerCounts(t *testing.T) {
	for _, configured := range []int{-1, 0, 1, 100} {
		t.Run(strconv.Itoa(configured), func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("autoconf_config_files_num_workers", configured)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "one.yaml"), []byte("instances: [{}]\n"), 0o600))
			r := &configFilesReader{paths: []string{dir}}
			recorder := startup.NewRecorder(true)
			recorder.BeginHook(1)
			phase := recorder.Start("initialize", "file")
			configs, _, errs := r.readWithTracing(GetAll, phase)
			phase.Finish(nil)
			require.Len(t, configs, 1)
			require.Empty(t, errs)
			events, _ := recorder.Drain()
			var pool *startup.Event
			for i := range events {
				if events[i].Name == "autodiscovery.config_files.read_parse" {
					pool = &events[i]
				}
			}
			require.NotNil(t, pool)
			require.Equal(t, float64(configured), pool.Metrics["config_files.workers_configured"])
			require.Equal(t, float64(1), pool.Metrics["config_files.workers_used"])
		})
	}
}

func TestConfigReaderStatsReadErrors(t *testing.T) {
	var stats configReadStats
	_, _, err := getIntegrationConfigFromFileWithStats("missing", filepath.Join(t.TempDir(), "absent.yaml"), &stats)
	require.Error(t, err)
	require.Equal(t, 1, stats.filesAttempted)
	require.Equal(t, 1, stats.readErrors)
	require.Zero(t, stats.filesRead)
	require.Zero(t, stats.configErrors)
	require.Zero(t, stats.parseNS)
}
