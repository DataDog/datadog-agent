// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logs contain methods to configure Agent logs
package logs

import (
	"errors"
	"io"
	stdslog "log/slog"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	seelogCfg "github.com/DataDog/datadog-agent/pkg/util/log/setup/internal/seelog"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

// LoggerName specifies the name of an instantiated logger.
type LoggerName string

// Constant values for LoggerName.
const (
	CoreLoggerName      LoggerName = "CORE"
	JMXLoggerName       LoggerName = "JMXFETCH"
	DogstatsDLoggerName LoggerName = "DOGSTATSD"
)

// SetupLogger sets up a logger with the specified logger name and log level
// if a non empty logFile is provided, it will also log to the file
// a non empty syslogURI will enable syslog, and format them following RFC 5424 if specified
// you can also specify to log to the console and in JSON format
func SetupLogger(loggerName LoggerName, logLevel, logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool, cfg pkgconfigmodel.Reader) error {
	seelogLogLevel, err := log.ValidateLogLevel(logLevel)
	if err != nil {
		return err
	}
	loggerInterface, err := buildLogger(loggerName, seelogLogLevel, logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, cfg)
	if err != nil {
		return err
	}
	handler := loggerInterface.(*slog.Wrapper).Handler()
	stdslog.SetDefault(stdslog.New(handler))
	log.SetupLogger(loggerInterface, seelogLogLevel.String())

	// Registering a callback in case of "log_level" update
	cfg.OnUpdate(func(setting string, _ pkgconfigmodel.Source, oldValue, newValue any, _ uint64) {
		if setting != "log_level" || oldValue == newValue {
			return
		}
		level := newValue.(string)

		seelogLogLevel, err := log.ValidateLogLevel(level)
		if err != nil {
			log.Warnf("Unable to set new log level: %v", err)
			return
		}
		loggerInterface, err := buildLogger(loggerName, seelogLogLevel, logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, cfg)
		if err != nil {
			return
		}
		handler := loggerInterface.(*slog.Wrapper).Handler()
		stdslog.SetDefault(stdslog.New(handler))
		// We wire the new logger with the Datadog logic
		log.ChangeLogLevel(loggerInterface, seelogLogLevel)
	})
	return nil
}

// BuildJMXLogger returns a logger with JMX logger name and log level
// if a non empty logFile is provided, it will also log to the file
// a non empty syslogURI will enable syslog, and format them following RFC 5424 if specified
// you can also specify to log to the console and in JSON format
func BuildJMXLogger(logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool, cfg pkgconfigmodel.Reader) (log.LoggerInterface, error) {
	// The JMX logger always logs at level "info", because JMXFetch does its
	// own level filtering on and provides all messages to seelog at the info
	// or error levels, via log.JMXInfo and log.JMXError.
	logger, err := buildLogger(JMXLoggerName, log.InfoLvl, logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, cfg)
	if err != nil {
		return nil, err
	}
	return logger, nil
}

// SetupDogstatsdLogger returns a logger with dogstatsd logger name and log level
// if a non empty logFile is provided, it will also log to the file
func SetupDogstatsdLogger(logFile string, cfg pkgconfigmodel.Reader) (log.LoggerInterface, error) {
	logger, err := buildDogstatsdLogger(DogstatsDLoggerName, log.InfoLvl, logFile, cfg)
	if err != nil {
		return nil, err
	}
	return logger, nil
}

func buildDogstatsdLogger(loggerName LoggerName, seelogLogLevel log.LogLevel, logFile string, cfg pkgconfigmodel.Reader) (log.LoggerInterface, error) {
	config := seelogCfg.NewSeelogConfig(string(loggerName), seelogLogLevel.String(), "common", false, nil, commonFormatter(loggerName, cfg))

	// Configuring max roll for log file, if dogstatsd_log_file_max_rolls env var is not set (or set improperly ) within datadog.yaml then default value is 3
	dogstatsdLogFileMaxRolls := cfg.GetInt("dogstatsd_log_file_max_rolls")
	if dogstatsdLogFileMaxRolls < 0 {
		dogstatsdLogFileMaxRolls = 3
		log.Warnf("Invalid value for dogstatsd_log_file_max_rolls, please make sure the value is equal or higher than 0")
	}

	// Configure log file, log file max size, log file roll up
	config.EnableFileLogging(logFile, cfg.GetSizeInBytes("dogstatsd_log_file_max_size"), uint(dogstatsdLogFileMaxRolls))

	return generateLoggerInterface(config, cfg)
}

func buildLogger(loggerName LoggerName, seelogLogLevel log.LogLevel, logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool, cfg pkgconfigmodel.Reader) (log.LoggerInterface, error) {
	formatID := "common"
	if jsonFormat {
		formatID = "json"
	}

	config := seelogCfg.NewSeelogConfig(string(loggerName), seelogLogLevel.String(), formatID, syslogRFC, jsonFormatter(loggerName, cfg), commonFormatter(loggerName, cfg))
	config.EnableConsoleLog(logToConsole)
	config.EnableFileLogging(logFile, cfg.GetSizeInBytes("log_file_max_size"), uint(cfg.GetInt("log_file_max_rolls")))

	if syslogURI != "" { // non-blank uri enables syslog
		config.ConfigureSyslog(syslogURI)
	}

	return generateLoggerInterface(config, cfg)
}

// generateLoggerInterface return a logger Interface from a log config
func generateLoggerInterface(logConfig *seelogCfg.Config, _ pkgconfigmodel.Reader) (log.LoggerInterface, error) {
	return logConfig.SlogLogger()
}

// logWriter is a Writer that logs all written messages with the global seelog logger
type logWriter struct {
	additionalDepth int
	logFunc         func(int, ...interface{})
}

// NewLogWriter returns a logWriter set with given logLevel. Returns an error if logLevel is unknown/not set.
func NewLogWriter(additionalDepth int, logLevel log.LogLevel) (io.Writer, error) {
	writer := &logWriter{
		additionalDepth: additionalDepth,
	}

	switch logLevel {
	case log.TraceLvl:
		writer.logFunc = log.TraceStackDepth
	case log.DebugLvl:
		writer.logFunc = log.DebugStackDepth
	case log.InfoLvl:
		writer.logFunc = log.InfoStackDepth
	case log.WarnLvl:
		writer.logFunc = func(dept int, v ...interface{}) {
			_ = log.WarnStackDepth(dept, v...)
		}
		writer.additionalDepth++
	case log.ErrorLvl:
		writer.logFunc = func(dept int, v ...interface{}) {
			_ = log.ErrorStackDepth(dept, v...)
		}
		writer.additionalDepth++
	case log.CriticalLvl:
		writer.logFunc = func(dept int, v ...interface{}) {
			_ = log.CriticalStackDepth(dept, v...)
		}
		writer.additionalDepth++
	default:
		return nil, errors.New("unknown loglevel in logwriter creation")
	}

	return writer, nil
}

func (s *logWriter) Write(p []byte) (n int, err error) {
	s.logFunc(s.additionalDepth, strings.TrimSpace(string(p)))
	return len(p), nil
}

const tlsHandshakeErrorKeyword = "http: TLS handshake error"

// tlsHandshakeErrorWriter writes TLS handshake errors to log with
// debug level, to avoid flooding of tls handshake errors.
type tlsHandshakeErrorWriter struct {
	writer io.Writer
}

// NewTLSHandshakeErrorWriter is a wrapper function which creates a new logWriter.
func NewTLSHandshakeErrorWriter(additionalDepth int, logLevel log.LogLevel) (io.Writer, error) {
	logWriter, err := NewLogWriter(additionalDepth, logLevel)
	if err != nil {
		return nil, err
	}
	tlsWriter := &tlsHandshakeErrorWriter{
		writer: logWriter,
	}
	return tlsWriter, nil
}

// Write writes TLS handshake errors to log with debug level.
func (t *tlsHandshakeErrorWriter) Write(p []byte) (n int, err error) {
	if strings.Contains(string(p), tlsHandshakeErrorKeyword) {
		log.DebugStackDepth(2, strings.TrimSpace(string(p)))
		return len(p), nil
	}
	return t.writer.Write(p)
}
