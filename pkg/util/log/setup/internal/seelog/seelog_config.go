// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seelog provides the configuration for the logger
package seelog

import (
	"bytes"
	"html/template"
	"strings"
	"sync"
)

// Config abstracts seelog XML configuration definition
type Config struct {
	settings map[string]interface{}
	sync.Mutex
}

const seelogConfigurationTemplate = `
<seelog minlevel="{{.logLevel}}">
	<outputs formatid="{{.format}}">
		{{if .consoleLoggingEnabled}}<console />{{end}}
		{{if .logfile              }}<rollingfile type="size" filename="{{.logfile}}" maxsize="{{.maxsize}}" maxrolls="{{.maxrolls}}" />{{end}}
		{{if .syslogURI            }}<custom name="syslog" formatid="syslog-{{.format}}" data-uri="{{.syslogURI}}" data-tls="{{.syslogUseTLS}}" />{{end}}
	</outputs>
	<formats>
		<format id="json"          format="{{.jsonFormat}}"/>
		<format id="common"        format="{{.commonFormat}}"/>
		<format id="syslog-json"   format="%CustomSyslogHeader(20,{{.syslogRFC}}) {{getJSONSyslogFormat .loggerName}}"/>
		<format id="syslog-common" format="%CustomSyslogHeader(20,{{.syslogRFC}}) {{.loggerName}} | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n" />
	</formats>
</seelog>`

func (c *Config) setValue(k string, v interface{}) {
	c.Lock()
	defer c.Unlock()
	c.settings[k] = v
}

// Render generates a string containing a valid seelog XML configuration
func (c *Config) Render() (string, error) {
	c.Lock()
	defer c.Unlock()
	funcMap := template.FuncMap{
		// This function will be called by the html/template engine that will perform HTML escaping of quotes characters in the output string
		"getJSONSyslogFormat": func(name string) string {
			return `{"agent":"` + strings.ToLower(name) + `","level":"%LEVEL","relfile":"%ShortFilePath","line":"%Line","msg":"%Msg"%ExtraJSONContext}%n`
		},
	}
	tmpl, err := template.New("seelog_config").Funcs(funcMap).Parse(seelogConfigurationTemplate)
	if err != nil {
		return "", err
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, c.settings)
	return b.String(), err
}

// EnableConsoleLog sets enable or disable console logging depending on the parameter value
func (c *Config) EnableConsoleLog(v bool) {
	c.setValue("consoleLoggingEnabled", v)
}

// SetLogLevel configures the loglevel
func (c *Config) SetLogLevel(l string) {
	c.setValue("logLevel", l)
}

// EnableFileLogging enables and configures file logging if the filename is not empty
func (c *Config) EnableFileLogging(f string, maxsize, maxrolls uint) {
	c.Lock()
	defer c.Unlock()
	c.settings["logfile"] = f
	c.settings["maxsize"] = maxsize
	c.settings["maxrolls"] = maxrolls
}

// ConfigureSyslog enables and configures syslog if the syslogURI it not an empty string
func (c *Config) ConfigureSyslog(syslogURI string, usetTLS bool) {
	c.Lock()
	defer c.Unlock()
	c.settings["syslogURI"] = template.URL(syslogURI)
	c.settings["syslogUseTLS"] = usetTLS

}

// NewSeelogConfig returns a SeelogConfig filled with correct parameters
func NewSeelogConfig(name, level, format, jsonFormat, commonFormat string, syslogRFC bool) *Config {
	c := &Config{settings: make(map[string]interface{})}
	c.settings["loggerName"] = name
	c.settings["format"] = format
	c.settings["syslogRFC"] = syslogRFC
	c.settings["jsonFormat"] = jsonFormat
	c.settings["commonFormat"] = commonFormat
	c.settings["logLevel"] = level
	return c
}
