// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || darwin

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSyslogURI(t *testing.T) {
	assert := assert.New(t)

	mockConfig := Mock(t)
	mockConfig.Set("log_to_syslog", true)
	mockConfig.Set("syslog_uri", "")

	assert.Equal(GetSyslogURI(), defaultSyslogURI)

	mockConfig.Set("syslog_uri", "tcp://localhost:514")
	assert.Equal(GetSyslogURI(), "tcp://localhost:514")

	mockConfig.Set("log_to_syslog", false)
	assert.Equal(GetSyslogURI(), "")

	mockConfig.Set("syslog_uri", "")
	assert.Equal(GetSyslogURI(), "")
}

func TestSetupLoggingNowhere(t *testing.T) {
	// setup logger so that it logs nowhere: i.e.  not to file, not to syslog, not to console
	seelogConfig, _ = buildLoggerConfig("agent", "info", "", "", false, false, false)
	loggerInterface, err := GenerateLoggerInterface(seelogConfig)

	assert.Nil(t, loggerInterface)
	assert.NotNil(t, err)
}
