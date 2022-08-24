// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"github.com/cihub/seelog"
	"github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var seeLogToLogrusLevel = map[seelog.LogLevel]logrus.Level{
	seelog.TraceLvl:    logrus.TraceLevel,
	seelog.DebugLvl:    logrus.DebugLevel,
	seelog.InfoLvl:     logrus.InfoLevel,
	seelog.WarnLvl:     logrus.WarnLevel,
	seelog.ErrorLvl:    logrus.ErrorLevel,
	seelog.CriticalLvl: logrus.FatalLevel,
}

// GetLogrusLevel returns logrus log level from log.GetLogLevel()
func GetLogrusLevel() *logrus.Logger {
	logLevel, err := log.GetLogLevel()
	if err != nil {
		log.Warnf("error getting log level")
	}
	logrusLevel, ok := seeLogToLogrusLevel[logLevel]
	if !ok {
		log.Warnf("no matching logrus level for seelog level: %s", logLevel.String())
		logrusLevel = logrus.InfoLevel
	}
	logger := logrus.StandardLogger()
	logger.SetLevel(logrusLevel)
	return logger
}
