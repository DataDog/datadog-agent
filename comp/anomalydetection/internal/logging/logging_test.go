// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"bytes"
	"testing"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func TestGlobalLoggerPrefixesFormattedAndUnformattedMessages(t *testing.T) {
	t.Cleanup(func() { pkglog.SetLogObserver(nil) })

	var messages []string
	pkglog.SetLogObserver(func(_ pkglog.LogLevel, message string) {
		messages = append(messages, message)
	})

	Infof("configured %d detectors", 2)
	Warn("logssource", "unavailable")

	require.Equal(t, []string{
		"[anomalydetection] configured 2 detectors",
		"[anomalydetection] logssource unavailable",
	}, messages)
}

func TestGlobalLoggerPreservesCallerAttribution(t *testing.T) {
	var output bytes.Buffer
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndFullFormat(&output, pkglog.InfoLvl)
	require.NoError(t, err)
	t.Cleanup(func() {
		pkglog.SetupLogger(pkglog.Default(), pkglog.InfoStr)
		logger.Close()
	})
	pkglog.SetupLogger(logger, pkglog.InfoStr)

	emitAttributionProbe()
	logger.Flush()

	logs := output.String()
	require.Contains(t, logs, "emitAttributionProbe")
	require.NotContains(t, logs, "logging.go")
}

func emitAttributionProbe() {
	Infof("caller attribution probe")
}
