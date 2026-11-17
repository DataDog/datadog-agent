// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package crashreport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

func TestCrashCheckpointMigration(t *testing.T) {
	cfg := configmock.New(t)
	runDir := t.TempDir()
	cfg.SetInTest("run_path", runDir)
	parent := fmt.Sprintf(`Software\DatadogCrashReportTest-%d`, time.Now().UnixNano())
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, parent) })

	for _, name := range []string{"agent_crash_reporting", "windows_crash_reporting"} {
		t.Run(name, func(t *testing.T) {
			path := parent + `\` + name
			t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, path) })
			checkpointKey := name + ":lastReported"
			reporter, err := NewWinCrashReporter(checkpointKey, registry.CURRENT_USER, path)
			require.NoError(t, err)
			legacy := probe.WinCrashStatus{FileName: name + "_dump.dmp", DateString: "old"}
			newCrash := probe.WinCrashStatus{FileName: name + "_dump.dmp", DateString: "new"}

			// An absent file and registry key must not create any checkpoint.
			require.False(t, reporter.haveAlreadyReported(legacy))
			require.Equal(t, name+":lastReported", reporter.checkpointKey)
			require.False(t, persistentcache.Exists(reporter.checkpointKey))
			// GetFileForKey creates the directory for colon-separated keys,
			// but a missing checkpoint must not create the file itself.
			_, err = os.Stat(filepath.Join(runDir, name, "lastReported"))
			require.True(t, os.IsNotExist(err))

			key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
			require.NoError(t, err)
			require.NoError(t, key.SetStringValue(reportedKey, legacyReportedValue(legacy)))
			require.NoError(t, key.Close())

			// An unmatched legacy marker should not be imported as the new
			// crash's checkpoint. It will be replaced when that crash is reported.
			require.False(t, reporter.haveAlreadyReported(newCrash))
			require.False(t, persistentcache.Exists(reporter.checkpointKey))

			// A matching legacy checkpoint is imported without reporting again.
			require.True(t, reporter.haveAlreadyReported(legacy))
			value, err := persistentcache.Read(reporter.checkpointKey)
			require.NoError(t, err)
			var saved crashCheckpoint
			require.NoError(t, json.Unmarshal([]byte(value), &saved))
			require.Equal(t, crashCheckpoint{
				Version: checkpointVersion, FileName: legacy.FileName, DateString: legacy.DateString,
			}, saved)
			_, err = os.Stat(filepath.Join(runDir, name, "lastReported"))
			require.NoError(t, err)

			// An empty file has no usable checkpoint; fall back to the legacy
			// marker and replace it with structured state when it matches.
			require.NoError(t, persistentcache.WriteAtomic(reporter.checkpointKey, ""))
			require.True(t, reporter.haveAlreadyReported(legacy))
			value, err = persistentcache.Read(reporter.checkpointKey)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal([]byte(value), &saved))
			require.Equal(t, legacy.FileName, saved.FileName)

			// The file wins over stale registry state, and new writes never touch
			// the legacy key. This also covers a restart with a new reporter.
			reporter.setReported(newCrash)
			value, err = persistentcache.Read(reporter.checkpointKey)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal([]byte(value), &saved))
			require.Equal(t, crashCheckpoint{
				Version: checkpointVersion, FileName: newCrash.FileName, DateString: newCrash.DateString,
			}, saved)

			// Future metadata does not invalidate fields this reader understands.
			saved.Version = 42
			futureValue, err := json.Marshal(saved)
			require.NoError(t, err)
			require.NoError(t, persistentcache.WriteAtomic(reporter.checkpointKey, string(futureValue)))
			require.True(t, reporter.haveAlreadyReported(newCrash))
			require.NoError(t, persistentcache.WriteAtomic(reporter.checkpointKey, `{"file_name":"`+newCrash.FileName+`"}`))
			require.False(t, reporter.haveAlreadyReported(newCrash), "missing date_string cannot match")
			require.NoError(t, persistentcache.WriteAtomic(reporter.checkpointKey, string(futureValue)))

			restarted, err := NewWinCrashReporter(checkpointKey, registry.CURRENT_USER, path)
			require.NoError(t, err)
			require.True(t, restarted.haveAlreadyReported(newCrash))
			require.False(t, restarted.haveAlreadyReported(legacy))
			key, err = registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
			require.NoError(t, err)
			value, _, err = key.GetStringValue(reportedKey)
			require.NoError(t, err)
			require.Equal(t, legacyReportedValue(legacy), value)
			require.NoError(t, key.Close())
			require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, path))
			require.True(t, restarted.haveAlreadyReported(newCrash), "file state survives removal of legacy registry key")
		})
	}
}
