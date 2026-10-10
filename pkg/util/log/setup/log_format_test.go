// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package logs

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJMXFetchJSONFormatter(t *testing.T) {
	format := jsonFormatter(JMXLoggerName, nil) // the JMXFetch formatter does not read the config
	record := func(msg string) slog.Record {
		return slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
	}

	t.Run("structured line", func(t *testing.T) {
		out := format(context.Background(), record("2026-10-02 08:51:14 UTC | JMX | INFO | main | StatsdReporter | Starting \"reporter\""))

		var got map[string]string
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, map[string]string{
			"agent":  "jmxfetch",
			"time":   "2026-10-02 08:51:14 UTC",
			"level":  "INFO",
			"thread": "main",
			"logger": "StatsdReporter",
			"msg":    `Starting "reporter"`,
		}, got)
	})

	t.Run("padded level", func(t *testing.T) {
		out := format(context.Background(), record("2026-10-02 08:51:14 UTC | JMX | WARN  | main | App | a | b"))

		var got map[string]string
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, "WARN", got["level"])
		assert.Equal(t, "a | b", got["msg"])
	})

	t.Run("fields are trimmed", func(t *testing.T) {
		out := format(context.Background(), record("2026-10-02 08:51:14 UTC  | JMX | INFO | main  | App  | hello"))

		var got map[string]string
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, "2026-10-02 08:51:14 UTC", got["time"])
		assert.Equal(t, "main", got["thread"])
		assert.Equal(t, "App", got["logger"])
	})

	t.Run("line without thread", func(t *testing.T) {
		out := format(context.Background(), record("2026-10-02 08:51:14 UTC | JMX | INFO | StatsdReporter | Starting"))

		var got map[string]string
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, map[string]string{
			"agent":  "jmxfetch",
			"time":   "2026-10-02 08:51:14 UTC",
			"level":  "INFO",
			"logger": "StatsdReporter",
			"msg":    "Starting",
		}, got)
	})

	t.Run("unstructured line falls back to msg only", func(t *testing.T) {
		out := format(context.Background(), record("Exception in thread main"))
		assert.JSONEq(t, `{"msg":"Exception in thread main"}`, out)
	})

	t.Run("malformed lines fall back to msg only", func(t *testing.T) {
		for _, msg := range []string{
			"2026-10-02 08:51:14 UTC | JMX | INFO | App",                    // missing sections
			"2026-10-02 08:51:14 UTC | JMX",                                 // truncated
			"2026-10-02 08:51:14 UTC | OTHER | INFO | main | App | message", // wrong marker
		} {
			out := format(context.Background(), record(msg))
			want, err := json.Marshal(map[string]string{"msg": msg})
			require.NoError(t, err)
			assert.JSONEq(t, string(want), out, msg)
		}
	})
}
