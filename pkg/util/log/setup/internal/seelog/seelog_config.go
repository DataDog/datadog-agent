// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seelog provides the configuration for the logger
package seelog

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	stdslog "log/slog"
	"os"
	"strings"
	"sync"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/filewriter"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/log/syslog"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// Config abstracts seelog XML configuration definition
type Config struct {
	sync.Mutex

	consoleLoggingEnabled bool
	logLevel              string
	logfile               string
	maxsize               uint
	maxrolls              uint
	syslogURI             string
	loggerName            string
	format                string
	syslogRFC             bool
	// seelog format strings
	jsonFormat   string
	commonFormat string
	// slog formatters, should produce the same output as the seelog format strings
	jsonFormatter   func(ctx context.Context, r stdslog.Record) string
	commonFormatter func(ctx context.Context, r stdslog.Record) string
}

const seelogConfigurationTemplate = `
<seelog minlevel="%[1]s">
	<outputs formatid="%[2]s">
		%[3]s
		%[4]s
		%[5]s
	</outputs>
	<formats>
		<format id="json"          format="%[6]s"/>
		<format id="common"        format="%[7]s"/>
		<format id="syslog-json"   format="%%CustomSyslogHeader(20,%[8]t) %[9]s"/>
		<format id="syslog-common" format="%%CustomSyslogHeader(20,%[8]t) %[10]s | %%LEVEL | (%%ShortFilePath:%%Line in %%FuncShort) | %%ExtraTextContext%%Msg%%n" />
	</formats>
</seelog>`

// Render generates a string containing a valid seelog XML configuration
func (c *Config) Render() (string, error) {
	c.Lock()
	defer c.Unlock()

	var consoleLoggingEnabled string
	if c.consoleLoggingEnabled {
		consoleLoggingEnabled = "<console />"
	}

	var logfile string
	if c.logfile != "" {
		logfile = fmt.Sprintf(`<rollingfile type="size" filename="%s" maxsize="%d" maxrolls="%d" />`, xmlEscape(c.logfile), c.maxsize, c.maxrolls)
	}

	var syslogURI string
	if c.syslogURI != "" {
		syslogURI = fmt.Sprintf(`<custom name="syslog" formatid="syslog-%s" data-uri="%s" />`, xmlEscape(c.format), xmlEscape(c.syslogURI))
	}

	jsonSyslogFormat := xmlEscape(`{"agent":"` + strings.ToLower(c.loggerName) + `","level":"%LEVEL","relfile":"%ShortFilePath","line":"%Line","msg":"%Msg"%ExtraJSONContext}%n`)

	return fmt.Sprintf(seelogConfigurationTemplate, xmlEscape(c.logLevel), xmlEscape(c.format), consoleLoggingEnabled, logfile, syslogURI, c.jsonFormat, c.commonFormat, c.syslogRFC, jsonSyslogFormat, xmlEscape(c.loggerName)), nil
}

// SlogLogger returns a slog logger behaving the same way as Render would configure a seelog logger
func (c *Config) SlogLogger() (types.LoggerInterface, error) {
	c.Lock()
	defer c.Unlock()

	if !c.consoleLoggingEnabled && c.logfile == "" && c.syslogURI == "" {
		// seelog requires at least one output to be configured, we do the same
		return nil, errors.New("no logging configuration provided")
	}

	// the logger:
	// - writes to stdout if consoleLoggingEnabled is true
	// - writes to the logfile if logfile is not empty
	// - writes to syslog if syslogURI is not empty

	var closeFuncs []func()

	// console writer
	var writers []io.Writer
	if c.consoleLoggingEnabled {
		writers = append(writers, os.Stdout)
	}

	// file writer
	if c.logfile != "" {
		fw, err := filewriter.NewRollingFileWriterSize(c.logfile, int64(c.maxsize), int(c.maxrolls), filewriter.RollingNameModePostfix)
		if err != nil {
			return nil, err
		}
		writers = append(writers, fw)
		closeFuncs = append(closeFuncs, func() { fw.Close() })
	}

	// main formatter using the writers
	var handlerList []stdslog.Handler
	if len(writers) > 0 {
		formatter := c.commonFormatter
		if c.format == "json" {
			formatter = c.jsonFormatter
		}
		handlerList = append(handlerList, handlers.NewFormat(formatter, io.MultiWriter(writers...)))
	}

	// syslog handler (formatter + writer)
	if c.syslogURI != "" {
		syslogReceiver := syslog.Receiver{}
		err := syslogReceiver.AfterParse(seelog.CustomReceiverInitArgs{
			XmlCustomAttrs: map[string]string{
				"uri": c.syslogURI,
			},
		})
		if err != nil {
			return nil, err
		}
		syslogFormatter := c.commonSyslogFormatter
		if c.format == "json" {
			syslogFormatter = c.jsonSyslogFormatter
		}
		handlerList = append(handlerList, handlers.NewFormat(syslogFormatter, &syslogReceiver))
		closeFuncs = append(closeFuncs, func() { syslogReceiver.Close() })
	}

	if c.logfile != "" {
		// create a seelog logger which only logs to a file
		// for debugging purposes
		cfg := Config{
			logfile:      c.logfile + ".seelog",
			maxsize:      c.maxsize,
			maxrolls:     c.maxrolls,
			loggerName:   c.loggerName,
			format:       c.format,
			jsonFormat:   c.jsonFormat,
			commonFormat: c.commonFormat,
			logLevel:     c.logLevel,
		}
		cfgStr, err := cfg.Render()
		if err == nil {
			seelogLogger, err := seelog.LoggerFromConfigAsString(cfgStr)
			if err == nil {
				handlerList = append(handlerList, &seelogHandler{handler: seelogLogger})
				closeFuncs = append(closeFuncs, func() { seelogLogger.Close() })
				_ = seelog.ReplaceLogger(seelogLogger)
			}
		}
	}

	// level handler -> async handler -> multi handler
	multiHandler := handlers.NewMulti(handlerList...)
	asyncHandler := handlers.NewAsync(multiHandler)
	closeFuncs = append(closeFuncs, asyncHandler.Close)

	lvl, err := log.ValidateLogLevel(c.logLevel)
	if err != nil {
		return nil, err
	}
	levelHandler := handlers.NewLevel(types.ToSlogLevel(lvl), asyncHandler)

	closeFunc := func() {
		for _, closeFunc := range closeFuncs {
			closeFunc()
		}
	}

	logger := slog.NewWrapperWithCloseAndFlush(levelHandler, asyncHandler.Flush, closeFunc)

	return logger, nil
}

type seelogHandler struct {
	handler seelog.LoggerInterface
}

//nolint:revive
func (h *seelogHandler) Handle(_ context.Context, r stdslog.Record) error {
	switch types.FromSlogLevel(r.Level) {
	case types.TraceLvl:
		h.handler.Trace(r.Message)
	case types.DebugLvl:
		h.handler.Debug(r.Message)
	case types.InfoLvl:
		h.handler.Info(r.Message)
	case types.WarnLvl:
		_ = h.handler.Warn(r.Message)
	case types.ErrorLvl:
		_ = h.handler.Error(r.Message)
	case types.CriticalLvl:
		_ = h.handler.Critical(r.Message)
	case types.Off:
	default:
		return fmt.Errorf("unknown log level: %d", r.Level)
	}
	return nil
}

//nolint:revive
func (h *seelogHandler) WithAttrs([]stdslog.Attr) stdslog.Handler {
	return h
}

//nolint:revive
func (h *seelogHandler) WithGroup(string) stdslog.Handler {
	return h
}

//nolint:revive
func (h *seelogHandler) Enabled(context.Context, stdslog.Level) bool {
	return true
}

// commonSyslogFormatter formats the syslog message in the common format
//
// It is equivalent to the seelog format string
// %CustomSyslogHeader(20,<syslog-rfc>) <logger-name> | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n
func (c *Config) commonSyslogFormatter(_ context.Context, r stdslog.Record) string {
	syslogHeaderFormatter := syslog.HeaderFormatter(20, c.syslogRFC)
	syslogHeader := syslogHeaderFormatter(r.Message, seelog.LogLevel(types.FromSlogLevel(r.Level)), nil)

	frame := formatters.Frame(r)
	level := formatters.CapitalizedLevel(r.Level)
	shortFilePath := formatters.ShortFilePath(frame)
	funcShort := formatters.ShortFunction(frame)
	extraContext := formatters.ExtraTextContext(r)

	return fmt.Sprintf("%s %s | %s | (%s:%d in %s) | %s%s\n", syslogHeader, c.loggerName, level, shortFilePath, frame.Line, funcShort, extraContext, r.Message)
}

// jsonSyslogFormatter formats the syslog message in the JSON format
//
// It is equivalent to the seelog format string
// %CustomSyslogHeader(20,<syslog-rfc>) {"agent":"<lowercase-logger-name>","level":"%LEVEL","relfile":"%ShortFilePath","line":"%Line","msg":"%Msg"%ExtraJSONContext}%n
func (c *Config) jsonSyslogFormatter(_ context.Context, r stdslog.Record) string {
	syslogHeaderFormatter := syslog.HeaderFormatter(20, c.syslogRFC)
	syslogHeader := syslogHeaderFormatter(r.Message, seelog.LogLevel(types.FromSlogLevel(r.Level)), nil)

	frame := formatters.Frame(r)
	level := formatters.UppercaseLevel(r.Level)
	relfile := formatters.RelFile(frame)
	extraContext := formatters.ExtraJSONContext(r)

	return fmt.Sprintf(`%s {"agent":"%s","level":"%s","relfile":"%s","line":"%d","msg":"%s"%s}`+"\n", syslogHeader, strings.ToLower(c.loggerName), level, relfile, frame.Line, r.Message, extraContext)
}

// EnableConsoleLog sets enable or disable console logging depending on the parameter value
func (c *Config) EnableConsoleLog(v bool) {
	c.Lock()
	defer c.Unlock()
	c.consoleLoggingEnabled = v
}

// SetLogLevel configures the loglevel
func (c *Config) SetLogLevel(l string) {
	c.Lock()
	defer c.Unlock()
	c.logLevel = l
}

// EnableFileLogging enables and configures file logging if the filename is not empty
func (c *Config) EnableFileLogging(f string, maxsize, maxrolls uint) {
	c.Lock()
	defer c.Unlock()
	c.logfile = f
	c.maxsize = maxsize
	c.maxrolls = maxrolls
}

// ConfigureSyslog enables and configures syslog if the syslogURI it not an empty string
func (c *Config) ConfigureSyslog(syslogURI string) {
	c.Lock()
	defer c.Unlock()
	c.syslogURI = syslogURI

}

// NewSeelogConfig returns a SeelogConfig filled with correct parameters
func NewSeelogConfig(name, level, format, jsonFormat, commonFormat string, syslogRFC bool, jsonFormatter, commonFormatter func(ctx context.Context, r stdslog.Record) string) *Config {
	c := &Config{}
	c.loggerName = name
	c.format = format
	c.syslogRFC = syslogRFC
	c.jsonFormat = xmlEscape(jsonFormat)
	c.jsonFormatter = jsonFormatter
	c.commonFormat = xmlEscape(commonFormat)
	c.commonFormatter = commonFormatter
	c.logLevel = level
	return c
}

func xmlEscape(in string) string {
	var buffer bytes.Buffer
	// EscapeText can only fail if writing to the buffer fails, and writing to a bytes.Buffer cannot fail
	_ = xml.EscapeText(&buffer, []byte(in))
	return buffer.String()
}
