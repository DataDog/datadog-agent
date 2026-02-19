// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || darwin

package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestGetSyslogURI(t *testing.T) {
	assert := assert.New(t)

	mockConfig := configmock.New(t)

	mockConfig.SetInTest("log_to_syslog", true)
	mockConfig.SetInTest("syslog_uri", "")

	assert.Equal(GetSyslogURI(mockConfig), defaultSyslogURI)

	mockConfig.SetInTest("syslog_uri", "tcp://localhost:514")
	assert.Equal(GetSyslogURI(mockConfig), "tcp://localhost:514")

	mockConfig.SetInTest("log_to_syslog", false)
	assert.Equal(GetSyslogURI(mockConfig), "")

	mockConfig.SetInTest("syslog_uri", "")
	assert.Equal(GetSyslogURI(mockConfig), "")
}

func TestSetupLoggingNowhere(t *testing.T) {
	// setup logger so that it logs nowhere: i.e.  not to file, not to syslog, not to console
	mockConfig := configmock.New(t)
	loggerInterface, err := buildLogger("agent", log.InfoLvl, "", "", false, false, false, mockConfig)

	assert.Nil(t, loggerInterface)
	assert.NotNil(t, err)
}
