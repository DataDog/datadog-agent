// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package file

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
)

const openFlagsFailureMessage = "Fingerprinting with the configured open flags failed for this file"

var errDirectIORejected = errors.New("direct I/O rejected")

// TestFingerprintOpenFlagsErrorSurfacesOnSource covers an initial failure before
// a tailer exists. Without a source-level message, the file would disappear from
// status with no explanation for why it was not scheduled.
func TestFingerprintOpenFlagsErrorSurfacesOnSource(t *testing.T) {
	reporter := newOpenFlagsErrorReporter()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.FileType, Path: "/logs/*.log"})
	first := filetailer.NewFile("/logs/a.log", source, true)
	second := filetailer.NewFile("/logs/b.log", source, true)

	reporter.report(first, errDirectIORejected)
	reporter.report(second, errDirectIORejected)

	reported := source.GetInfoStatus()[fingerprintOpenFlagsInfoKey]
	require.Len(t, reported, 2, "every failing file of a source reports under one heading")
	require.Contains(t, strings.Join(reported, "\n"), openFlagsFailureMessage)
	require.Contains(t, strings.Join(reported, "\n"), "direct I/O rejected")

	// Reporting the same file again replaces its message rather than adding one,
	// which matters because this runs on every scan.
	reporter.report(first, errDirectIORejected)
	require.Len(t, source.GetInfoStatus()[fingerprintOpenFlagsInfoKey], 2)
}

// TestResetFingerprintOpenFlagsErrors covers the scan scoping: a scan retracts
// what the previous one recorded, on every source it wrote to, so a file that
// recovered or disappeared leaves nothing behind.
func TestResetFingerprintOpenFlagsErrors(t *testing.T) {
	reporter := newOpenFlagsErrorReporter()
	// Two sources, because a failure found on an active tailer is recorded
	// against the tailer's own source, which need not be one the scan walked.
	scanned := sources.NewLogSource("scanned", &config.LogsConfig{Type: config.FileType, Path: "/logs/*.log"})
	unscanned := sources.NewLogSource("unscanned", &config.LogsConfig{Type: config.FileType, Path: "/new/*.log"})

	reporter.report(filetailer.NewFile("/logs/a.log", scanned, true), errDirectIORejected)
	reporter.report(filetailer.NewFile("/new/c.log", unscanned, true), errDirectIORejected)

	reporter.reset()

	_, present := scanned.GetInfoStatus()[fingerprintOpenFlagsInfoKey]
	require.False(t, present, "the heading disappears once no file of the source is failing")
	_, present = unscanned.GetInfoStatus()[fingerprintOpenFlagsInfoKey]
	require.False(t, present, "a source the scan did not walk is retracted too")

	// A scan that recorded nothing still resets. Most scans are that case.
	require.NotPanics(t, reporter.reset)
}
