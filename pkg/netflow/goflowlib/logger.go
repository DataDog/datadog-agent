// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-agent/comp/core/log"
)

var ddLogToLogrusLevel = map[log.Level]logrus.Level{
	log.TraceLvl:    logrus.TraceLevel,
	log.DebugLvl:    logrus.DebugLevel,
	log.InfoLvl:     logrus.InfoLevel,
	log.WarnLvl:     logrus.WarnLevel,
	log.ErrorLvl:    logrus.ErrorLevel,
	log.CriticalLvl: logrus.FatalLevel,
}

// GetLogrusLevel returns logrus log level from log.GetLogLevel()
func GetLogrusLevel(logger log.Component) *logrus.Logger {
	logLevel, err := logger.GetLogLevel()
	if err != nil {
		logger.Warnf("error getting log level")
	}
	logrusLevel, ok := ddLogToLogrusLevel[logLevel]
	if !ok {
		logger.Warnf("no matching logrus level for seelog level: %s", logLevel.String())
		logrusLevel = logrus.InfoLevel
	}
	logrusLogger := logrus.StandardLogger()
	logrusLogger.SetLevel(logrusLevel)
	return logrusLogger
}
