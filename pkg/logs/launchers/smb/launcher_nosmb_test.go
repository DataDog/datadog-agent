// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && !smb

// These tests cover the stub in builds without the smb tag only, not in FIPS
// builds (smb plus a FIPS tag): for those, Gazelle would add a test variant per
// FIPS tag, each rebuilding the whole dependency graph of the package's tests,
// for the one branch the builtForFIPS seam already covers.

package smb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func newSMBSource(name string) *sources.LogSource {
	return sources.NewLogSource(name, &config.LogsConfig{
		Type: config.SMBType,
		Path: "app/*.log",
		SMB: &config.SMBConfig{
			Host:     "fileserver.example.com",
			Share:    "logs",
			Username: "myacct",
			Password: "not-a-real-password",
		},
	})
}

// startLauncher starts l on logSources and returns once both the source added
// before the subscription and the one added after it have their status.
func startLauncher(t *testing.T, l *Launcher) (replayed, added *sources.LogSource) {
	t.Helper()
	logSources := sources.NewLogSources()
	replayed = newSMBSource("replayed")
	logSources.AddSource(replayed)

	l.Start(logSources, nil, nil, nil)
	t.Cleanup(l.Stop)
	added = newSMBSource("added")
	logSources.AddSource(added)

	for _, source := range []*sources.LogSource{replayed, added} {
		require.Eventually(t, source.Status().IsError, 5*time.Second, time.Millisecond, "status of %s", source.Name)
	}
	// The launcher does not subscribe to removals, so this must not block.
	logSources.RemoveSource(added)
	return replayed, added
}

func TestLauncherReportsSMBSourcesAsUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name         string
		builtForFIPS bool
		want         string
	}{
		{name: "build without the SMB log source", builtForFIPS: false, want: "does not include the SMB log source, which is available in the full Agent"},
		{name: "FIPS build", builtForFIPS: true, want: "not supported in FIPS builds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := NewLauncher(time.Second)
			l.builtForFIPS = func() bool { return tc.builtForFIPS }

			replayed, added := startLauncher(t, l)

			assert.Contains(t, replayed.Status().GetError(), tc.want)
			assert.Contains(t, added.Status().GetError(), tc.want)
		})
	}
}

func TestLauncherReportsTheReasonOfThisBuild(t *testing.T) {
	want := errNotIncluded
	if fips.BuiltForFIPS() {
		want = errFIPS
	}

	replayed, _ := startLauncher(t, NewLauncher(time.Second))

	assert.Equal(t, "Error: "+want.Error(), replayed.Status().GetError())
}

func TestLauncherStopWithoutStart(_ *testing.T) {
	l := NewLauncher(time.Second)
	l.Stop()
	l.Stop()
}
