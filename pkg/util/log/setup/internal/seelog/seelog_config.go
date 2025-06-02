// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seelog provides the configuration for the logger
package seelog

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
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
	syslogUseTLS          bool
	loggerName            string
	format                string
	syslogRFC             bool
	jsonFormat            string
	commonFormat          string
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
		logfile = fmt.Sprintf(`<rollingfile type="size" filename="%s" maxsize="%d" maxrolls="%d" />`, c.logfile, c.maxsize, c.maxrolls)
	}

	var syslogURI string
	if c.syslogURI != "" {
		syslogURI = fmt.Sprintf(`<custom name="syslog" formatid="syslog-%s" data-uri="%s" data-tls="%t" />`, c.format, c.syslogURI, c.syslogUseTLS)
	}

	jsonSyslogFormat := xmlEscape(`{"agent":"` + strings.ToLower(c.loggerName) + `","level":"%LEVEL","relfile":"%ShortFilePath","line":"%Line","msg":"%Msg"%ExtraJSONContext}%n`)

	return fmt.Sprintf(seelogConfigurationTemplate, c.logLevel, c.format, consoleLoggingEnabled, logfile, syslogURI, c.jsonFormat, c.commonFormat, c.syslogRFC, jsonSyslogFormat, xmlEscape(c.loggerName)), nil
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
	c.logfile = xmlEscape(f)
	c.maxsize = maxsize
	c.maxrolls = maxrolls
}

// ConfigureSyslog enables and configures syslog if the syslogURI it not an empty string
func (c *Config) ConfigureSyslog(syslogURI string, usetTLS bool) {
	c.Lock()
	defer c.Unlock()
	c.syslogURI = xmlEscape(syslogURI)
	c.syslogUseTLS = usetTLS

}

// NewSeelogConfig returns a SeelogConfig filled with correct parameters
func NewSeelogConfig(name, level, format, jsonFormat, commonFormat string, syslogRFC bool) *Config {
	c := &Config{}
	c.loggerName = name
	c.format = xmlEscape(format)
	c.syslogRFC = syslogRFC
	c.jsonFormat = xmlEscape(jsonFormat)
	c.commonFormat = xmlEscape(commonFormat)
	c.logLevel = xmlEscape(level)
	return c
}

func xmlEscape(in string) string {
	var buffer bytes.Buffer
	// EscapeText can only fail if writing to the buffer fails, and writing to a bytes.Buffer cannot fail
	_ = xml.EscapeText(&buffer, []byte(in))
	return buffer.String()
}
