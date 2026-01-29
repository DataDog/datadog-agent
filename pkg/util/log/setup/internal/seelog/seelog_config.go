// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seelog provides the configuration for the logger
package seelog

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdslog "log/slog"
	"os"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/filewriter"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/log/syslog"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// Config abstracts the logger configuration
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
	// slog formatters
	jsonFormatter   func(ctx context.Context, r stdslog.Record) string
	commonFormatter func(ctx context.Context, r stdslog.Record) string
}

// SlogLogger returns a slog logger
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
		handlerList = append(handlerList, handlers.NewFormat(formatter, newSplitWriter(writers...)))
	}

	// syslog handler (formatter + writer)
	if c.syslogURI != "" {
		syslogReceiver, err := syslog.NewReceiver(c.syslogURI)
		if err != nil {
			return nil, err
		}
		syslogFormatter := c.commonSyslogFormatter
		if c.format == "json" {
			syslogFormatter = c.jsonSyslogFormatter
		}
		handlerList = append(handlerList, handlers.NewFormat(syslogFormatter, syslogReceiver))
		closeFuncs = append(closeFuncs, func() { syslogReceiver.Close() })
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

// commonSyslogFormatter formats the syslog message in the common format
//
// It is equivalent to the seelog format string
// %CustomSyslogHeader(20,<syslog-rfc>) <logger-name> | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n
func (c *Config) commonSyslogFormatter(_ context.Context, r stdslog.Record) string {
	syslogHeaderFormatter := syslog.HeaderFormatter(20, c.syslogRFC)
	syslogHeader := syslogHeaderFormatter(types.FromSlogLevel(r.Level))

	frame := formatters.Frame(r)
	level := formatters.UppercaseLevel(r.Level)
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
	syslogHeader := syslogHeaderFormatter(types.FromSlogLevel(r.Level))

	frame := formatters.Frame(r)
	level := formatters.UppercaseLevel(r.Level)
	relfile := formatters.ShortFilePath(frame)
	extraContext := formatters.ExtraJSONContext(r)

	return fmt.Sprintf(`%s {"agent":"%s","level":"%s","relfile":"%s","line":"%d","msg":%s%s}`+"\n", syslogHeader, strings.ToLower(c.loggerName), level, relfile, frame.Line, formatters.Quote(r.Message), extraContext)
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

// NewSeelogConfig returns a Config filled with correct parameters
func NewSeelogConfig(name, level, format string, syslogRFC bool, jsonFormatter, commonFormatter func(ctx context.Context, r stdslog.Record) string) *Config {
	c := &Config{}
	c.loggerName = name
	c.format = format
	c.syslogRFC = syslogRFC
	c.jsonFormatter = jsonFormatter
	c.commonFormatter = commonFormatter
	c.logLevel = level
	return c
}
