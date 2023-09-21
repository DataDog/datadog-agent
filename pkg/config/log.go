package config

import (
	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config/logsetup"
)

type LoggerName = logsetup.LoggerName

const JMXLoggerName = logsetup.JMXLoggerName

var (
	ChangeLogLevel = logsetup.ChangeLogLevel
	NewLogWriter   = logsetup.NewLogWriter
)

func SetupLogger(loggerName LoggerName, logLevel, logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool) error {
	return logsetup.SetupLogger(loggerName, logLevel, logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, Datadog)
}

func GetSyslogURI() string {
	return logsetup.GetSyslogURI(Datadog)
}

func SetupDogstatsdLogger(logFile string) (seelog.LoggerInterface, error) {
	return logsetup.SetupDogstatsdLogger(logFile, Datadog)
}

func SetupJMXLogger(logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool) error {
	return logsetup.SetupJMXLogger(logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, Datadog)
}
