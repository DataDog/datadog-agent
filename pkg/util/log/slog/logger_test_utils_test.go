// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package slog

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

func TestLoggerFromWriterWithMinLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := LoggerFromWriterWithMinLevel(&buf, types.DebugLvl)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("test message")
	logger.Flush()
	if buf.Len() == 0 {
		t.Error("expected log output")
	}
}
