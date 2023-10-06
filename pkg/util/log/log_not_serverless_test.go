// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package log

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestServerlessLoggingNotInServerlessContext(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevel(w, seelog.DebugLvl)
	assert.Nil(t, err)

	SetupLogger(l, "debug")
	assert.NotNil(t, Logger)

	DebugfServerless("%s %d", "foo", 10)
	DebugServerless("Not in serverless mode")
	w.Flush()

	// Nothing is logged since we are not in a serverless context
	assert.Equal(t, 0, len(b.String()))
}
