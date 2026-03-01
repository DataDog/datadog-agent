// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package jmxloggerimpl implements the logger for JMX.
package jmxloggerimpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestJMXLog(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "jmx_test.log")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	assert.NoError(t, err)
	defer f.Close()

	reqs := Requires{
		Config: config.NewMock(t),
		Params: NewCliParams(filePath),
	}

	provides, err := NewComponent(reqs)
	assert.NoError(t, err)

	jmxLogger := provides.Comp

	jmxLogger.JMXError("jmx error message")
	jmxLogger.JMXInfo("jmx info message")

	jmxLogger.Flush()

	jmxLoggerInternal := jmxLogger.(logger)
	jmxLoggerInternal.close()

	bytes, err := os.ReadFile(filePath)
	assert.NoError(t, err)

	assert.Subset(t, strings.Split(string(bytes), "\n"), []string{
		"jmx error message",
		"jmx info message",
	})
}
