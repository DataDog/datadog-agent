// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logs contain methods to configure Agent logs
package logs

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/cihub/seelog"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	seelogCfg "github.com/DataDog/datadog-agent/pkg/util/log/setup/internal/seelog"
	"github.com/DataDog/datadog-agent/pkg/util/log/syslog"
)

// LoggerName specifies the name of an instantiated logger.
type LoggerName string

// Constant values for LoggerName.
const (
	CoreLoggerName      LoggerName = "CORE"
	JMXLoggerName       LoggerName = "JMXFETCH"
	DogstatsDLoggerName LoggerName = "DOGSTATSD"
)

type contextFormat uint8

const (
	jsonFormat = contextFormat(iota)
	textFormat
	logDateFormat = "2006-01-02 15:04:05 MST" // see time.Format for format syntax
)

var (
	seelogConfig          *seelogCfg.Config
	jmxSeelogConfig       *seelogCfg.Config
	dogstatsdSeelogConfig *seelogCfg.Config
)

func getLogDateFormat(cfg pkgconfigmodel.Reader) string {
	if cfg.GetBool("log_format_rfc3339") {
		return time.RFC3339
	}
	return logDateFormat
}

func createQuoteMsgFormatter(_ string) seelog.FormatterFunc {
	return func(message string, _ seelog.LogLevel, _ seelog.LogContextInterface) interface{} {
		return strconv.Quote(message)
	}
}

// SetupLogger sets up a logger with the specified logger name and log level
// if a non empty logFile is provided, it will also log to the file
// a non empty syslogURI will enable syslog, and format them following RFC 5424 if specified
// you can also specify to log to the console and in JSON format
func SetupLogger(loggerName LoggerName, logLevel, logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool, cfg pkgconfigmodel.Reader) error {
	seelogLogLevel, err := log.ValidateLogLevel(logLevel)
	if err != nil {
		return err
	}
	seelogConfig, err = buildLoggerConfig(loggerName, seelogLogLevel, logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, cfg)
	if err != nil {
		return err
	}
	loggerInterface, err := GenerateLoggerInterface(seelogConfig)
	if err != nil {
		return err
	}
	_ = seelog.ReplaceLogger(loggerInterface)
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
		// We create a new logger to propagate the new log level everywhere seelog is used (including dependencies)
		seelogConfig.SetLogLevel(seelogLogLevel.String())
		configTemplate, err := seelogConfig.Render()
		if err != nil {
			return
		}

		logger, err := seelog.LoggerFromConfigAsString(configTemplate)
		if err != nil {
			return
		}
		_ = seelog.ReplaceLogger(logger)
		// We wire the new logger with the Datadog logic
		log.ChangeLogLevel(logger, seelogLogLevel)
	})
	return nil
}

// BuildJMXLogger sets up a logger with JMX logger name and log level
// if a non empty logFile is provided, it will also log to the file
// a non empty syslogURI will enable syslog, and format them following RFC 5424 if specified
// you can also specify to log to the console and in JSON format
func BuildJMXLogger(logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool, cfg pkgconfigmodel.Reader) (seelog.LoggerInterface, error) {
	// The JMX logger always logs at level "info", because JMXFetch does its
	// own level filtering on and provides all messages to seelog at the info
	// or error levels, via log.JMXInfo and log.JMXError.
	var err error
	jmxSeelogConfig, err = buildLoggerConfig(JMXLoggerName, log.InfoLvl, logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, cfg)
	if err != nil {
		return nil, err
	}
	return GenerateLoggerInterface(jmxSeelogConfig)
}

// SetupDogstatsdLogger sets up a logger with dogstatsd logger name and log level
// if a non empty logFile is provided, it will also log to the file
func SetupDogstatsdLogger(logFile string, cfg pkgconfigmodel.Reader) (seelog.LoggerInterface, error) {
	dogstatsdSeelogConfig = buildDogstatsdLoggerConfig(DogstatsDLoggerName, log.InfoLvl, logFile, cfg)

	dogstatsdLoggerInterface, err := GenerateLoggerInterface(dogstatsdSeelogConfig)
	if err != nil {
		return nil, err
	}

	return dogstatsdLoggerInterface, nil
}

func buildDogstatsdLoggerConfig(loggerName LoggerName, seelogLogLevel log.LogLevel, logFile string, cfg pkgconfigmodel.Reader) *seelogCfg.Config {
	config := seelogCfg.NewSeelogConfig(string(loggerName), seelogLogLevel.String(), "common", "", buildCommonFormat(loggerName, cfg), false)

	// Configuring max roll for log file, if dogstatsd_log_file_max_rolls env var is not set (or set improperly ) within datadog.yaml then default value is 3
	dogstatsdLogFileMaxRolls := cfg.GetInt("dogstatsd_log_file_max_rolls")
	if dogstatsdLogFileMaxRolls < 0 {
		dogstatsdLogFileMaxRolls = 3
		log.Warnf("Invalid value for dogstatsd_log_file_max_rolls, please make sure the value is equal or higher than 0")
	}

	// Configure log file, log file max size, log file roll up
	config.EnableFileLogging(logFile, cfg.GetSizeInBytes("dogstatsd_log_file_max_size"), uint(dogstatsdLogFileMaxRolls))

	return config
}

func buildLoggerConfig(loggerName LoggerName, seelogLogLevel log.LogLevel, logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool, cfg pkgconfigmodel.Reader) (*seelogCfg.Config, error) {
	formatID := "common"
	if jsonFormat {
		formatID = "json"
	}

	config := seelogCfg.NewSeelogConfig(string(loggerName), seelogLogLevel.String(), formatID, buildJSONFormat(loggerName, cfg), buildCommonFormat(loggerName, cfg), syslogRFC)
	config.EnableConsoleLog(logToConsole)
	config.EnableFileLogging(logFile, cfg.GetSizeInBytes("log_file_max_size"), uint(cfg.GetInt("log_file_max_rolls")))

	if syslogURI != "" { // non-blank uri enables syslog
		config.ConfigureSyslog(syslogURI)
	}
	return config, nil
}

// GenerateLoggerInterface return a logger Interface from a log config
func GenerateLoggerInterface(logConfig *seelogCfg.Config) (seelog.LoggerInterface, error) {
	configTemplate, err := logConfig.Render()
	if err != nil {
		return nil, err
	}

	loggerInterface, err := seelog.LoggerFromConfigAsString(configTemplate)
	if err != nil {
		return nil, err
	}

	return loggerInterface, nil
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

func parseShortFilePath(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return extractShortPathFromFullPath(context.FullPath())
	}
}

func extractShortPathFromFullPath(fullPath string) string {
	shortPath := ""
	if strings.Contains(fullPath, "-agent/") {
		// We want to trim the part containing the path of the project
		// ie DataDog/datadog-agent/ or DataDog/datadog-process-agent/
		slices := strings.Split(fullPath, "-agent/")
		shortPath = slices[len(slices)-1]
	} else {
		// For logging from dependencies, we want to log e.g.
		// "collector@v0.35.0/service/collector.go"
		slices := strings.Split(fullPath, "/")
		atSignIndex := len(slices) - 1
		for ; atSignIndex > 0; atSignIndex-- {
			if strings.Contains(slices[atSignIndex], "@") {
				break
			}
		}
		shortPath = strings.Join(slices[atSignIndex:], "/")
	}
	return shortPath
}

func createExtraJSONContext(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		contextList, ok := context.CustomContext().([]interface{})
		if len(contextList) == 0 || !ok {
			return ""
		}
		return extractContextString(jsonFormat, contextList)
	}
}

func createExtraTextContext(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		contextList, ok := context.CustomContext().([]interface{})
		if len(contextList) == 0 || !ok {
			return ""
		}
		return extractContextString(textFormat, contextList)
	}
}

func extractContextString(format contextFormat, contextList []interface{}) string {
	if len(contextList) == 0 || len(contextList)%2 != 0 {
		return ""
	}

	builder := strings.Builder{}
	if format == jsonFormat {
		builder.WriteString(",")
	}

	for i := 0; i < len(contextList); i += 2 {
		key, val := contextList[i], contextList[i+1]
		// Only add if key is string
		if keyStr, ok := key.(string); ok {
			addToBuilder(&builder, keyStr, val, format, i == len(contextList)-2)
		}
	}

	if format != jsonFormat {
		builder.WriteString(" | ")
	}

	return builder.String()
}

func addToBuilder(builder *strings.Builder, key string, value interface{}, format contextFormat, isLast bool) {
	var buf []byte
	appendFmt(builder, format, key, buf)
	builder.WriteString(":")
	switch val := value.(type) {
	case string:
		appendFmt(builder, format, val, buf)
	default:
		appendFmt(builder, format, fmt.Sprintf("%v", val), buf)
	}
	if !isLast {
		builder.WriteString(",")
	}
}

func appendFmt(builder *strings.Builder, format contextFormat, s string, buf []byte) {
	if format == jsonFormat {
		buf = buf[:0]
		buf = strconv.AppendQuote(buf, s)
		builder.Write(buf)
	} else {
		builder.WriteString(s)
	}
}

func init() {
	_ = seelog.RegisterCustomFormatter("CustomSyslogHeader", syslog.CreateSyslogHeaderFormatter)
	_ = seelog.RegisterCustomFormatter("ShortFilePath", parseShortFilePath)
	_ = seelog.RegisterCustomFormatter("ExtraJSONContext", createExtraJSONContext)
	_ = seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
	seelog.RegisterReceiver("syslog", &syslog.Receiver{})
}
