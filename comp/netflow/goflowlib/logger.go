// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"github.com/sirupsen/logrus"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"
)

var ddLogToLogrusLevel = map[ddlog.LogLevel]logrus.Level{
	ddlog.TraceLvl:    logrus.TraceLevel,
	ddlog.DebugLvl:    logrus.DebugLevel,
	ddlog.InfoLvl:     logrus.InfoLevel,
	ddlog.WarnLvl:     logrus.WarnLevel,
	ddlog.ErrorLvl:    logrus.ErrorLevel,
	ddlog.CriticalLvl: logrus.FatalLevel,
}

// GetLogrusLevel returns logrus log level from log.GetLogLevel()
func GetLogrusLevel(logger log.Component) *logrus.Logger {
	// TODO: ideally this would be exposed by the log component but there were
	// some issues getting #19033 merged. Right now this will always be the
	// datadog log level, even if you pass in a different logger. This problem
	// will also go away when we upgrade to the latest goflow2, as we will no
	// longer need to interact with logrus.
	logLevel, err := ddlog.GetLogLevel()
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
