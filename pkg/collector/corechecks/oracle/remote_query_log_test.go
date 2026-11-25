// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle && test

package oracle

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OBS-03
func TestOBS03_LogWithoutData(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, log.DebugLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "debug")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "info") })

	f := newRemoteFixture(t)
	f.expectQuery(probeRows([2]any{"424242", "secret-row-value"}))
	assert.JSONEq(t, `{"status":"SUCCEEDED"}`, f.execute(t, nil))
	f = newRemoteFixture(t)
	requireFailure(t, f.execute(t, withQuery("DROP TABLE rq_probe")), codeUnsupportedQuery)
	log.Flush()
	require.NoError(t, w.Flush())

	var runLines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "oracle remote query") {
			runLines = append(runLines, line)
		}
	}
	require.Len(t, runLines, 3, buf.String())
	for _, line := range runLines {
		for _, id := range []string{"crawl_id=" + testCrawlID, "task_id=" + testTaskID, "upload_id=" + testUploadID} {
			assert.Contains(t, line, id)
		}
		for _, secret := range []string{testQuery, "rq_probe", "424242", "secret-row-value", testPassword, testAPIKey, "db.example.com"} {
			assert.NotContains(t, line, secret)
		}
	}
	assert.Contains(t, runLines[0], "[DEBUG]")
	for _, field := range []string{"page=0", "rows=1", "sent_bytes=", "encoding=identity", "compress_ms=", "upload_ms=", "gzip_not_smaller=true"} {
		assert.Contains(t, runLines[0], field)
	}
	assert.Contains(t, runLines[1], "[INFO]")
	assert.Contains(t, runLines[1], "outcome=succeeded code= pages=1 rows=1")
	assert.Contains(t, runLines[2], "outcome=failed code=unsupported_query")
}
