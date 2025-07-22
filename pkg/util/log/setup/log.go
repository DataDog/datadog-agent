// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logs contain methods to configure Agent logs
package logs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/filewriter"
	"github.com/DataDog/datadog-agent/pkg/util/log/format"
	"github.com/DataDog/datadog-agent/pkg/util/log/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/log/syslog"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
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
	lvl, err := log.ValidateLogLevel(logLevel)
	if err != nil {
		return err
	}
	logger, err := buildLogger(loggerName, lvl, logFile, int64(cfg.GetSizeInBytes("log_file_max_size")), cfg.GetInt("log_file_max_rolls"), syslogURI, syslogRFC, logToConsole, jsonFormat, cfg)
	if err != nil {
		return err
	}
	slog.SetDefault(logger.Logger())
	log.SetupLogger(logger, logLevel)

	// Registering a callback in case of "log_level" update
	cfg.OnUpdate(func(setting string, oldValue, newValue any, _ uint64) {
		if setting != "log_level" || oldValue == newValue {
			return
		}
		level := newValue.(string)

		lvl, err := log.ValidateLogLevel(level)
		if err != nil {
			log.Warnf("Unable to set new log level: %v", err)
			return
		}

		logger.SetLevel(lvl)
	})
	return nil
}

// SetupJMXLogger sets up a logger with JMX logger name and log level
// if a non empty logFile is provided, it will also log to the file
// a non empty syslogURI will enable syslog, and format them following RFC 5424 if specified
// you can also specify to log to the console and in JSON format
func SetupJMXLogger(logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool, cfg pkgconfigmodel.Reader) error {
	// The JMX logger always logs at level "info", because JMXFetch does its
	// own level filtering on and provides all messages to seelog at the info
	// or error levels, via log.JMXInfo and log.JMXError.
	lvl, err := log.ValidateLogLevel("info")
	if err != nil {
		return err
	}
	logger, err := buildLogger(JMXLoggerName, lvl, logFile, int64(cfg.GetSizeInBytes("log_file_max_size")), cfg.GetInt("log_file_max_rolls"), syslogURI, syslogRFC, logToConsole, jsonFormat, cfg)
	if err != nil {
		return err
	}
	log.SetupJMXLogger(logger, "info")
	return nil
}

// SetupDogstatsdLogger sets up a logger with dogstatsd logger name and log level
// if a non empty logFile is provided, it will also log to the file
func SetupDogstatsdLogger(logFile string, cfg pkgconfigmodel.Reader) (log.LoggerInterface, error) {
	seelogLogLevel, err := log.ValidateLogLevel("info")
	if err != nil {
		return nil, err
	}

	return buildDogstatsdLogger(DogstatsDLoggerName, seelogLogLevel, logFile, cfg)
}

func buildDogstatsdLogger(loggerName LoggerName, logLevel log.LogLevel, logFile string, cfg pkgconfigmodel.Reader) (*log.SlogWrapper, error) {
	// Configuring max roll for log file, if dogstatsd_log_file_max_rolls env var is not set (or set improperly ) within datadog.yaml then default value is 3
	dogstatsdLogFileMaxRolls := cfg.GetInt("dogstatsd_log_file_max_rolls")
	if dogstatsdLogFileMaxRolls < 0 {
		dogstatsdLogFileMaxRolls = 3
		log.Warnf("Invalid value for dogstatsd_log_file_max_rolls, please make sure the value is equal or higher than 0")
	}

	logger, err := buildLogger(loggerName, logLevel, logFile, int64(cfg.GetSizeInBytes("dogstatsd_log_file_max_size")), dogstatsdLogFileMaxRolls, "", false, false, false, cfg)
	return logger, err
}

func buildLogger(loggerName LoggerName, logLevel log.LogLevel, logFile string, logFileMaxSize int64, logFileMaxRolls int, syslogURI string, syslogRFC, logToConsole, jsonFormat bool, cfg pkgconfigmodel.Reader) (*log.SlogWrapper, error) {
	var logFormatter func(ctx context.Context, record slog.Record) string
	if jsonFormat {
		logFormatter = format.JSON(string(loggerName), syslogRFC)
	} else {
		logFormatter = format.Text(string(loggerName), syslogRFC)
	}

	logHandlers := []slog.Handler{}
	closeFunctions := []func() error{}

	if logFile != "" {
		rollingfilewriter, err := filewriter.NewRollingFileWriterSize(logFile, logFileMaxSize, logFileMaxRolls, filewriter.RollingNameModePostfix)
		if err != nil {
			return nil, err
		}
		fileHandler := handlers.NewFormatHandler(logFormatter, rollingfilewriter)
		logHandlers = append(logHandlers, fileHandler)
		closeFunctions = append(closeFunctions, rollingfilewriter.Close)
	}

	if logToConsole {
		consoleHandler := handlers.NewFormatHandler(logFormatter, os.Stdout)
		logHandlers = append(logHandlers, consoleHandler)
	}

	if syslogURI != "" {
		syslogReceiver, err := syslog.NewReceiver(string(loggerName), jsonFormat, syslogURI, cfg.GetString("syslog_pem"), cfg.GetString("syslog_key"), syslogRFC, cfg.GetBool("syslog_tls_verify"))
		if err != nil {
			return nil, err
		}
		logHandlers = append(logHandlers, syslogReceiver.Handler())
		closeFunctions = append(closeFunctions, syslogReceiver.Close)
	}

	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.Level(logLevel))

	multiHandler := handlers.NewMultiHandler(logHandlers...)
	scrubberHandler := handlers.NewScrubberHandler(scrubber.DefaultScrubber, multiHandler)
	asyncHandler := handlers.NewAsyncHandler(scrubberHandler)
	closeFunctions = append(closeFunctions, asyncHandler.Close)
	levelHandler := handlers.NewLevelHandler(levelVar, asyncHandler)

	logger := slog.New(levelHandler)

	close := func() error {
		var errs []error
		for _, closeFunction := range closeFunctions {
			if err := closeFunction(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}

	slogWrapper := log.NewSlogWrapper(logger, 2, levelVar, asyncHandler.Flush, close)

	return slogWrapper, nil
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
