// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build serverless

package log

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestServerlessLoggingInServerlessContext(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %ExtraTextContext%Msg\n")
	assert.Nil(t, err)

	SetupLogger(l, "debug")
	assert.NotNil(t, logger)

	DebugfServerless("%s %d", "foo", 10)
	DebugServerless("In serverless mode")
	w.Flush()

	assert.Equal(t, "[DEBUG] DebugfServerless: foo 10\n[DEBUG] DebugServerless: In serverless mode\n", b.String())
}
