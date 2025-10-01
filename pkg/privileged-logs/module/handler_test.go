// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestLogFileAccess(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.InfoLvl, "%Msg\n")
	require.NoError(t, err)
	log.SetupLogger(l, "info")

	module := NewPrivilegedLogsModule().(*privilegedLogsModule)

	paths := []string{
		"/var/log/app1.log",
		"/var/log/app2.log",
		"/var/log/app3.log",
	}

	for _, path := range paths {
		module.logFileAccess(path)
	}
	w.Flush()

	output := b.String()
	b.Reset()
	for _, path := range paths {
		assert.Contains(t, output, "Received request to open file: "+path,
			"each unique path should be logged once")
		assert.Equal(t, 1, strings.Count(output, path),
			"each path should appear exactly once")
	}

	for _, path := range paths {
		module.logFileAccess(path)
	}
	w.Flush()

	assert.Equal(t, 0, b.Len(), "repeated access should not log again")
	b.Reset()

	for i := 0; i < 128; i++ {
		module.logFileAccess(fmt.Sprintf("/var/log/file%d.log", i))
	}
	module.logFileAccess(paths[0])
	w.Flush()

	assert.Contains(t, b.String(), "Received request to open file: "+paths[0], "after LRU eviction, first path should be logged again")
	b.Reset()
}
