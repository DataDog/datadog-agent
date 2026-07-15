// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package jmxloggerimpl implements the logger for JMX.
package jmxloggerimpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	jmxlogger "github.com/DataDog/datadog-agent/comp/agent/jmxlogger/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

func TestJMXLog(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "jmx_test.log")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	assert.NoError(t, err)
	defer f.Close()

	provides, err := NewComponent(Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Config: config.NewMock(t),
		Params: jmxlogger.NewCliParams(filePath),
	})

	assert.NoError(t, err)

	provides.Comp.JMXError("jmx error message")
	provides.Comp.JMXInfo("jmx info message")

	provides.Comp.Flush()

	jmxLoggerInternal := provides.Comp.(logger)
	jmxLoggerInternal.close()

	bytes, err := os.ReadFile(filePath)
	assert.NoError(t, err)

	assert.Subset(t, strings.Split(string(bytes), "\n"), []string{
		"jmx error message",
		"jmx info message",
	})
}
