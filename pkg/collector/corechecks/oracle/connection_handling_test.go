// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"bytes"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFailingConnection(t *testing.T) {
	c, _ := newDbDoesNotExistCheck(t, "", "")
	c.Run()
}

func TestFailingConnectionDoesNotLeak(t *testing.T) {
	countOpeners := func() int {
		var buf bytes.Buffer
		require.NoError(t, pprof.Lookup("goroutine").WriteTo(&buf, 2))
		return strings.Count(buf.String(), "database/sql.(*DB).connectionOpener(")
	}

	connectConfig := getConnectData(t, useDefaultUser)
	connectConfig.ServiceName = doesNotExist
	c, _ := newTestCheck(t, connectConfig, "", "")

	before := countOpeners()
	for i := 0; i < 10; i++ {
		_, err := c.Connect()
		require.Error(t, err, "expected connection to a nonexistent service to fail")
	}

	// A leaked connectionOpener goroutine never exits; allow a brief settle
	// for the successful Close path.
	require.Eventually(t, func() bool {
		return countOpeners() <= before
	}, 5*time.Second, 50*time.Millisecond, "Connect() leaked connectionOpener goroutines across 10 failed attempts")
}
