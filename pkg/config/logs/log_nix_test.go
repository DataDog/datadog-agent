// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || darwin

package logs

import (
	"strings"
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
)

func TestGetSyslogURI(t *testing.T) {
	assert := assert.New(t)

	mockConfig := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	mockConfig.SetWithoutSource("log_to_syslog", true)
	mockConfig.SetWithoutSource("syslog_uri", "")

	assert.Equal(GetSyslogURI(mockConfig), defaultSyslogURI)

	mockConfig.SetWithoutSource("syslog_uri", "tcp://localhost:514")
	assert.Equal(GetSyslogURI(mockConfig), "tcp://localhost:514")

	mockConfig.SetWithoutSource("log_to_syslog", false)
	assert.Equal(GetSyslogURI(mockConfig), "")

	mockConfig.SetWithoutSource("syslog_uri", "")
	assert.Equal(GetSyslogURI(mockConfig), "")
}

func TestSetupLoggingNowhere(t *testing.T) {
	// setup logger so that it logs nowhere: i.e.  not to file, not to syslog, not to console
	mockConfig := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	seelogConfig, _ = buildLoggerConfig("agent", "info", "", "", false, false, false, mockConfig)
	loggerInterface, err := GenerateLoggerInterface(seelogConfig)

	assert.Nil(t, loggerInterface)
	assert.NotNil(t, err)
}
