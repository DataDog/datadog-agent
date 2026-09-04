// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pipelinemock "github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/testutils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// captureLauncherLogs redirects the Agent logger to a buffer for the duration of fn and returns
// everything it emitted at or above minLevel, one "[LEVEL] message" line per entry.
func captureLauncherLogs(t *testing.T, minLevel log.LogLevel, fn func()) string {
	t.Helper()

	var logBuffer bytes.Buffer
	logWriter := bufio.NewWriter(&logBuffer)
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(logWriter, minLevel)
	require.NoError(t, err)

	previousLogger := log.Default()
	t.Cleanup(func() {
		log.SetupLogger(previousLogger, "debug")
	})
	log.SetupLogger(logger, strings.ToLower(minLevel.String()))

	fn()

	require.NoError(t, logWriter.Flush())
	return logBuffer.String()
}

// countLines returns the number of lines in logs that contain substr.
func countLines(logs string, substr string) int {
	count := 0
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, substr) {
			count++
		}
	}
	return count
}

// fingerprintSkipTestSetup wires a launcher against a single fixed log path configured with the
// byte_checksum strategy, mirroring the configuration of a customer whose files rotate to a size
// smaller than the configured count.
type fingerprintSkipTestSetup struct {
	launcher *Launcher
	source   *sources.LogSource
	path     string
	scan     func()
}

func setupFingerprintSkipTest(t *testing.T, count int) *fingerprintSkipTestSetup {
	mockConfig := configmock.New(t)
	path := t.TempDir() + "/fingerprint-skip.log"

	fingerprintConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               count,
	}

	launcher := createLauncher(t, launcherTestOptions{
		openFilesLimit:    10,
		fingerprintConfig: fingerprintConfig,
	})
	launcher.pipelineProvider = pipelinemock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()

	source := sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              path,
		FingerprintConfig: fingerprintConfig,
	})
	launcher.activeSources = append(launcher.activeSources, source)

	status.Clear()
	status.InitStatus(mockConfig, testutils.CreateSources([]*sources.LogSource{source}))
	t.Cleanup(status.Clear)

	return &fingerprintSkipTestSetup{
		launcher: launcher,
		source:   source,
		path:     path,
		scan: func() {
			launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(
				context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))
		},
	}
}

// writeFile writes size bytes to path, replacing whatever was there.
func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", size)), 0644))
}

// That a file too short to fingerprint is not tailed is existing behaviour, covered by
// TestLauncherDoesNotCreateTailerForRotatedUndersizedFile. What matters here is that the skip is
// reported once when it starts rather than once per scan, since the launcher re-evaluates every
// scan period.
func TestLauncherWarnsOnceWhenFileIsTooShortToFingerprint(t *testing.T) {
	const (
		count = 2048
		scans = 5
	)
	setup := setupFingerprintSkipTest(t, count)
	writeFile(t, setup.path, count-1)

	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		for i := 0; i < scans; i++ {
			setup.scan()
		}
	})

	// One warning for the whole time the file stays skipped, not one per scan.
	assert.Equal(t, 1, countLines(logs, "is too short for fingerprinting"),
		"the skip must be warned about exactly once while the condition persists, got:\n%s", logs)
	assert.Contains(t, logs, setup.path, "the warning must name the file")
	// The threshold has to carry its unit: count means bytes here, but lines under line_checksum.
	assert.Contains(t, logs, "needs 2048 bytes", "the warning must report the threshold and its unit")

	skip, isSkipped := setup.launcher.fingerprintSkips[setup.path]
	require.True(t, isSkipped, "the launcher must remember it is skipping this file")
	assert.Equal(t, fingerprintSkipInsufficientData, skip.reason)
	assert.True(t, skip.warned[fingerprintSkipInsufficientData],
		"the warning must stay latched so later scans stay silent")
}

// Once the file grows past the configured count it is tailed again, and the recovery is logged so
// the collection gap can be bounded from the logs alone.
func TestLauncherLogsRecoveryAfterFingerprintSkip(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	writeFile(t, setup.path, 10000)
	setup.scan()
	require.NoError(t, os.Rename(setup.path, setup.path+".1"))
	writeFile(t, setup.path, 100)

	setup.scan()
	setup.scan()
	require.Equal(t, 0, setup.launcher.tailers.Count())

	// The application keeps writing until the file is long enough to fingerprint.
	writeFile(t, setup.path, count)

	logs := captureLauncherLogs(t, log.InfoLvl, func() {
		setup.scan()
	})

	assert.Equal(t, 1, setup.launcher.tailers.Count())
	assert.Contains(t, logs, "Now tailing "+setup.path)
	// Matched loosely because a unit test recovers in well under a second; what matters is that a
	// duration is rendered at all, not its value.
	assert.Regexp(t, `Now tailing .*, [0-9hms]+ after it was first skipped`, logs,
		"the recovery must report how long the file went untailed")
	assert.Empty(t, setup.launcher.fingerprintSkips, "the skip must be forgotten once the file is tailed")
}

// A fingerprint that could not be computed at all is reported differently from one that is merely
// unavailable for now: the fingerprinter reports both as an invalid fingerprint, so the error is
// what tells them apart.
func TestLauncherWarnsWhenFingerprintComputationFails(t *testing.T) {
	setup := setupFingerprintSkipTest(t, 2048)

	// The mock fingerprinter returns an error for any file it has no fingerprint set for.
	fingerprinter := filetailer.NewFingerprinterMock()
	fingerprinter.SetShouldFileFingerprint(filetailer.NewFile(setup.path, setup.source, false), true)
	setup.launcher.fingerprinter = fingerprinter
	writeFile(t, setup.path, 10000)

	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		setup.scan()
		setup.scan()
	})

	assert.Equal(t, 0, setup.launcher.tailers.Count())
	assert.Equal(t, 1, countLines(logs, "fingerprint could not be computed"),
		"the failure must be warned about exactly once, got:\n%s", logs)

	skip, isSkipped := setup.launcher.fingerprintSkips[setup.path]
	require.True(t, isSkipped)
	assert.Equal(t, fingerprintSkipError, skip.reason)
}

// Skip state must not outlive the files it refers to, otherwise a path that rotates away leaks an
// entry on every scan. Losing track of a file also stops the skipping, which is the case worth
// noticing: the file was never tailed at all.
func TestLauncherForgetsFingerprintSkipsForVanishedFiles(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	writeFile(t, setup.path, count-1)
	setup.scan()
	setup.scan()
	require.Len(t, setup.launcher.fingerprintSkips, 1)

	require.NoError(t, os.Remove(setup.path))
	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		setup.scan()
	})
	assert.Empty(t, setup.launcher.fingerprintSkips)

	// Giving up on a file needs a closing log line just like a recovery does, otherwise a file that
	// was never tailed leaves no record of when its gap ended.
	assert.Contains(t, logs, "Stopped tracking "+setup.path)
	assert.Regexp(t, `Stopped tracking .*, never tailed for [0-9hms]+ because`, logs,
		"the closing line must report how long the file went untailed")
}

// A shutdown is not the same event as a file we gave up on, so it is reported once for the whole
// set rather than once per file: an Agent stopped while many files were too short to fingerprint
// should not bury its own shutdown under a warning per file.
func TestLauncherReportsStillSkippedFilesOnceOnCleanup(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	writeFile(t, setup.path, count-1)
	setup.scan()
	require.Len(t, setup.launcher.fingerprintSkips, 1)

	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		setup.launcher.cleanup()
	})

	assert.Empty(t, setup.launcher.fingerprintSkips, "shutdown must not leave skip state behind")
	assert.Equal(t, 1, countLines(logs, "still not tailed because their fingerprint was unusable"),
		"shutdown must report the outstanding files as a single line, got:\n%s", logs)
	assert.NotContains(t, logs, "Stopped tracking "+setup.path,
		"shutdown must not be reported as giving up on the file")
}

// A file whose fingerprint starts failing for a different reason is still the same uninterrupted
// gap in collection, so the launcher carries on counting checks instead of starting over. Splitting
// it would report one long gap as two shorter ones.
func TestLauncherKeepsFingerprintSkipAcrossReasonChange(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	// First scan: the file is simply too short to fingerprint.
	writeFile(t, setup.path, count-1)
	setup.scan()

	skip, isSkipped := setup.launcher.fingerprintSkips[setup.path]
	require.True(t, isSkipped)
	require.Equal(t, fingerprintSkipInsufficientData, skip.reason)
	since := skip.since

	// The file now fails to be read at all. The mock errors for any file it has no fingerprint for.
	fingerprinter := filetailer.NewFingerprinterMock()
	fingerprinter.SetShouldFileFingerprint(filetailer.NewFile(setup.path, setup.source, false), true)
	setup.launcher.fingerprinter = fingerprinter

	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		setup.scan()
	})

	skip, isSkipped = setup.launcher.fingerprintSkips[setup.path]
	require.True(t, isSkipped)
	assert.Equal(t, fingerprintSkipError, skip.reason, "the new reason must be adopted")
	assert.Equal(t, since, skip.since, "the start time must span the whole gap, not restart")

	// The new reason is worth a fresh warning, since the remediation differs.
	assert.Equal(t, 1, countLines(logs, "fingerprint could not be computed"),
		"a new reason must be warned about, got:\n%s", logs)
}

// A new reason is worth a warning, but a file that keeps flipping between the two reasons must not
// warn on every scan: an intermittent read failure on a file that is also too short would otherwise
// defeat the latch entirely and report the same gap once per scan period.
func TestLauncherWarnsOncePerReasonWhenFingerprintFlaps(t *testing.T) {
	const (
		count  = 2048
		cycles = 3
	)
	setup := setupFingerprintSkipTest(t, count)
	writeFile(t, setup.path, count-1)

	// The real fingerprinter reports the short file as unusable with no error, while the mock
	// reports an error for any file it has no fingerprint for. Alternating between the two flips the
	// reason on every scan without the file ever becoming tailable.
	tooShort := setup.launcher.fingerprinter
	failing := filetailer.NewFingerprinterMock()
	failing.SetShouldFileFingerprint(filetailer.NewFile(setup.path, setup.source, false), true)

	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		for i := 0; i < cycles; i++ {
			setup.launcher.fingerprinter = failing
			setup.scan()
			setup.launcher.fingerprinter = tooShort
			setup.scan()
		}
	})

	assert.Equal(t, 1, countLines(logs, "fingerprint could not be computed"),
		"the error reason must be warned about once across every flip, got:\n%s", logs)
	assert.Equal(t, 1, countLines(logs, "is too short for fingerprinting"),
		"the too-short reason must be warned about once across every flip, got:\n%s", logs)
}

// A scan runs concurrently with the run loop against a copy of activeSources taken when it started,
// so its result cannot mention files whose source was added while it was in flight. Those files must
// not look like they vanished: reporting that we gave up on a file we are still retrying would split
// one continuous gap into two and restart the duration we report for it.
func TestLauncherKeepsFingerprintSkipOpenedDuringInFlightScan(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)
	writeFile(t, setup.path, count-1)

	// What FilesToTail returns for a scan that started before this source existed.
	staleResult := setup.launcher.fileProvider.FilesToTail(
		context.Background(), setup.launcher.validatePodContainerID, nil, setup.launcher.registry)
	require.Empty(t, staleResult, "a scan predating the source must not report its files")

	// The between-scans entry point, which addSource uses to evaluate a new source immediately
	// rather than waiting for the next scan.
	setup.launcher.launchTailers(setup.source)
	skip := setup.launcher.fingerprintSkips[setup.path]
	require.NotNil(t, skip, "the file must be skipped before the stale result lands")
	since := skip.since

	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		setup.launcher.resolveActiveTailers(staleResult)
	})

	require.Len(t, setup.launcher.fingerprintSkips, 1,
		"the skip must survive a scan result that predates its source")
	assert.NotContains(t, logs, "Stopped tracking",
		"a file we are still retrying must not be reported as given up on, got:\n%s", logs)
	assert.Equal(t, since, setup.launcher.fingerprintSkips[setup.path].since,
		"the gap must stay continuous rather than restarting")

	// And the file is still expired normally once it really does stop being expected.
	require.NoError(t, os.Remove(setup.path))
	setup.scan()
	assert.Empty(t, setup.launcher.fingerprintSkips, "a genuinely vanished file must still be expired")
}

// A scan returns at most open_files_limit files, so a result that reached the limit may have left
// out files that are still matched: a skipped file occupies one of those slots, and rotation renames
// files, which moves them across the limit boundary at the same moment their fingerprints become
// unusable. Reporting those as given up on would restart the gap we report for a file that every
// scan is still retrying.
func TestLauncherKeepsFingerprintSkipWhenScanHitFileLimit(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	writeFile(t, setup.path, count-1)
	setup.scan()
	skip := setup.launcher.fingerprintSkips[setup.path]
	require.NotNil(t, skip, "the file must be skipped first")
	since := skip.since

	// A result that filled every slot, and so cannot be read as everything that is matched.
	setup.launcher.tailingLimit = 1
	otherPath := t.TempDir() + "/other.log"
	writeFile(t, otherPath, count)
	otherSource := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: otherPath})
	atLimit := []*filetailer.File{filetailer.NewFile(otherPath, otherSource, false)}

	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		setup.launcher.resolveActiveTailers(atLimit)
	})

	require.Len(t, setup.launcher.fingerprintSkips, 1,
		"a file left out of a scan result that hit the limit must stay tracked")
	assert.NotContains(t, logs, "Stopped tracking",
		"a file we are still retrying must not be reported as given up on, got:\n%s", logs)
	assert.Equal(t, since, setup.launcher.fingerprintSkips[setup.path].since,
		"the gap must stay continuous rather than restarting")

	// A file that really is gone is still forgotten, so the map stays bounded for an Agent that sits
	// at its file limit for a long time.
	require.NoError(t, os.Remove(setup.path))
	setup.launcher.resolveActiveTailers(atLimit)
	assert.Empty(t, setup.launcher.fingerprintSkips,
		"a deleted file must be expired even from a scan result that hit the limit")
}

// When a scan hits open_files_limit, a skipped file left out of the result is kept only while its
// source is still active. Removing the source while the physical file remains must still abandon the
// skip, or the entry never clears, re-adds inherit stale duration and warning state, and dynamic
// source churn can grow fingerprintSkips without bound.
func TestLauncherAbandonsFingerprintSkipWhenSourceRemovedAtFileLimit(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	writeFile(t, setup.path, count-1)
	setup.scan()
	skip := setup.launcher.fingerprintSkips[setup.path]
	require.NotNil(t, skip, "the file must be skipped first")
	since := skip.since

	setup.launcher.tailingLimit = 1
	otherPath := t.TempDir() + "/other.log"
	writeFile(t, otherPath, count)
	otherSource := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: otherPath})
	atLimit := []*filetailer.File{filetailer.NewFile(otherPath, otherSource, false)}

	// Source goes away while the skipped file is still on disk and every scan is at the limit.
	setup.launcher.activeSources = nil

	logs := captureLauncherLogs(t, log.WarnLvl, func() {
		setup.launcher.resolveActiveTailers(atLimit)
	})

	assert.Empty(t, setup.launcher.fingerprintSkips,
		"removing the source must clear the skip even when the file still exists at the limit")
	assert.Contains(t, logs, "Stopped tracking "+setup.path,
		"source removal must close the gap, got:\n%s", logs)

	// Re-adding the path must start fresh rather than inherit the old gap.
	setup.launcher.activeSources = append(setup.launcher.activeSources, setup.source)
	setup.scan()
	freshSkip := setup.launcher.fingerprintSkips[setup.path]
	require.NotNil(t, freshSkip, "the file must be tracked again once the source returns")
	assert.True(t, freshSkip.since.After(since),
		"a re-added source must start a new gap rather than inherit the previous start time")

	logs = captureLauncherLogs(t, log.WarnLvl, func() {
		setup.scan()
		setup.scan()
	})
	assert.Equal(t, 0, countLines(logs, "is too short for fingerprinting"),
		"a re-added source must not inherit the previous warning latch, got:\n%s", logs)
}

// Customers cannot query the Agent's internal telemetry, so the reason a file is not being tailed
// has to reach them through `agent status`, next to the "N files tailed out of M matching" message
// that otherwise reports the shortfall without explaining it.
func TestLauncherReportsFingerprintSkipOnSourceStatus(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	writeFile(t, setup.path, count-1)
	setup.scan()

	messages := setup.source.Messages.GetMessages()
	require.Len(t, messages, 1, "the source must carry exactly one message for the skipped file")
	assert.Contains(t, messages[0], setup.path, "the message must name the file")
	assert.Contains(t, messages[0], "needs 2048 bytes", "the message must report the threshold and its unit")
	// This source carries its own fingerprint_config, which wins over the global one, so the global
	// setting is not what would make this file tailable.
	assert.Contains(t, messages[0], "this source's fingerprint_config.count",
		"the message must name the setting that actually applies")
	assert.NotContains(t, messages[0], "logs_config.fingerprint_config.count",
		"a per-source config cannot be fixed by changing the global setting")

	// The message must not outlive the problem, or status would keep accusing a healthy file.
	writeFile(t, setup.path, count)
	setup.scan()
	require.Equal(t, 1, setup.launcher.tailers.Count())
	assert.Empty(t, setup.source.Messages.GetMessages(),
		"the message must be cleared once the file is tailed")
}

// Remediation advice is only worth printing if it applies: the count a file fell short of can come
// from the source, from the global config, or from a built-in fallback that answers to no setting.
func TestFingerprintCountSetting(t *testing.T) {
	tests := []struct {
		name     string
		source   types.FingerprintConfigSource
		expected string
	}{
		{"per-source wins over global", types.FingerprintConfigSourcePerSource, "this source's fingerprint_config.count"},
		// GlobalFingerprintConfig unmarshals into a bare config, so the global case arrives unstamped.
		{"global", types.FingerprintConfigSourceGlobal, "logs_config.fingerprint_config.count"},
		{"unstamped global", "", "logs_config.fingerprint_config.count"},
		{"built-in fallback has no setting", types.FingerprintConfigSourceDefault, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fingerprint := &types.Fingerprint{Config: &types.FingerprintConfig{Source: test.source}}
			assert.Equal(t, test.expected, fingerprintCountSetting(fingerprint))
		})
	}

	// A file with no resolved config gives us nothing to point at, and a guess would be wrong.
	assert.Empty(t, fingerprintCountSetting(nil), "a missing fingerprint must not name a setting")
	assert.Empty(t, fingerprintCountSetting(&types.Fingerprint{}),
		"a fingerprint with no config must not name a setting")
}

// count is read only after seeking past count_to_skip, so a file larger than count can still be too
// short. Reporting count on its own would tell operators a file needs a size it already exceeds.
func TestFingerprintRequirementReportsSkippedData(t *testing.T) {
	requirement := func(strategy types.FingerprintStrategy, count, countToSkip int) string {
		return fingerprintRequirement(&types.Fingerprint{Config: &types.FingerprintConfig{
			FingerprintStrategy: strategy, Count: count, CountToSkip: countToSkip,
		}})
	}

	// count_to_skip defaults to 0, so the common case says nothing about it.
	assert.Equal(t, "2048 bytes", requirement(types.FingerprintStrategyByteChecksum, 2048, 0))
	assert.Equal(t, "5 lines", requirement(types.FingerprintStrategyLineChecksum, 5, 0))

	assert.Equal(t, "2048 bytes after the first 1000", requirement(types.FingerprintStrategyByteChecksum, 2048, 1000))
	assert.Equal(t, "5 lines after the first 3", requirement(types.FingerprintStrategyLineChecksum, 5, 3))

	assert.Equal(t, "more data", fingerprintRequirement(nil), "an unknown threshold must not invent a number")
}

// The file can only be measured with stat, which counts bytes. Reporting that size against a
// threshold in lines would compare two different units and tell operators nothing.
func TestFingerprintFileSizeMatchesThresholdUnit(t *testing.T) {
	path := t.TempDir() + "/measured.log"
	writeFile(t, path, 2047)

	size := func(strategy types.FingerprintStrategy) string {
		return fingerprintFileSize(path, &types.Fingerprint{
			Config: &types.FingerprintConfig{FingerprintStrategy: strategy},
		})
	}

	assert.Equal(t, "2047 bytes", size(types.FingerprintStrategyByteChecksum))
	assert.Equal(t, "The file", size(types.FingerprintStrategyLineChecksum),
		"a line threshold must not be measured against a byte count")

	// Best effort throughout: an unreadable or unknown file still has to report as not tailed.
	assert.Equal(t, "The file", fingerprintFileSize(path, nil))
	assert.Equal(t, "The file", fingerprintFileSize(t.TempDir()+"/missing.log",
		&types.Fingerprint{Config: &types.FingerprintConfig{FingerprintStrategy: types.FingerprintStrategyByteChecksum}}))
}

// A source can be removed and re-registered for the same path, by a config reload or by
// autodiscovery, while the file is still being skipped. The file keeps being skipped throughout, so
// the message has to follow onto the source that is now being displayed instead of staying on the
// one that is gone.
func TestLauncherMovesFingerprintSkipMessageToReplacementSource(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	writeFile(t, setup.path, count-1)
	setup.scan()
	require.Len(t, setup.source.Messages.GetMessages(), 1)

	// A second source for the same path takes over, as a config reload would produce.
	replacement := sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              setup.path,
		FingerprintConfig: &types.FingerprintConfig{FingerprintStrategy: types.FingerprintStrategyByteChecksum, Count: count},
	})
	setup.launcher.activeSources = []*sources.LogSource{replacement}

	setup.scan()

	require.Len(t, setup.launcher.fingerprintSkips, 1, "the file must still be tracked as skipped")
	messages := replacement.Messages.GetMessages()
	require.Len(t, messages, 1, "the replacement source must explain why the file is not tailed")
	assert.Contains(t, messages[0], setup.path)
	// The handover has to be a move, not a copy: a message left on the source that no longer matches
	// the file claims it is not being tailed for as long as that source is displayed.
	assert.Empty(t, setup.source.Messages.GetMessages(),
		"the message must be taken off the source that no longer matches the file")

	// And when the file stops being skipped, it must be cleared from the source that actually has it.
	writeFile(t, setup.path, count)
	setup.scan()
	require.Equal(t, 1, setup.launcher.tailers.Count())
	assert.Empty(t, replacement.Messages.GetMessages(),
		"the message must be cleared from the replacement source once the file is tailed")
}

// A launcher does not only stop when the Agent does: switching transport stops the launchers and
// keeps the sources, then builds new ones over them. The replacement launcher has no skip state to
// clear, so anything left on a source at stop is left there for the lifetime of the process.
func TestLauncherClearsFingerprintSkipMessagesOnStop(t *testing.T) {
	const count = 2048
	setup := setupFingerprintSkipTest(t, count)

	writeFile(t, setup.path, count-1)
	setup.scan()
	require.Len(t, setup.source.Messages.GetMessages(), 1, "the file must be reported as skipped first")

	setup.launcher.cleanup()

	assert.Empty(t, setup.launcher.fingerprintSkips, "stopping must drop the skip state")
	assert.Empty(t, setup.source.Messages.GetMessages(),
		"stopping must take the message off the source that outlives the launcher")
}
