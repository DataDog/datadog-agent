// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"bytes"
	"strings"
	"sync"
	"text/template"
)

// SeelogConfig abstract seelog XML configuration definition
type SeelogConfig struct {
	settings map[string]interface{}
	m        sync.Mutex
}

const seelogConfigurationTemplate = `
<seelog minlevel="{{.logLevel}}">
	<outputs formatid="{{.format}}">
    {{if .consoleLoggingEnabled   }}<console />{{end}}
    {{if .logfile                 }}<rollingfile type="size" filename="{{.logfile}}" maxsize="{{.maxsize}}" maxrolls="{{.maxrolls}}" />{{end}}
	{{if .syslogURI               }}<custom name="syslog" formatid="syslog-{{.format}}" data-uri="{{.syslogURI}}" data-tls="{{.syslogUseTLS}}" />{{end}}
	</outputs>
	<formats>
		<format id="json" format="{{.jsonFormat}}"/>
		<format id="common" format="{{.commonFormat}}"/>
		<format id="syslog-json" format="%CustomSyslogHeader(20,{{.syslogRFC}}){&quot;agent&quot;:&quot;{{.loggerName | ToLower}}&quot;,&quot;level&quot;:&quot;%LEVEL&quot;,&quot;relfile&quot;:&quot;%ShortFilePath&quot;,&quot;line&quot;:&quot;%Line&quot;,&quot;msg&quot;:&quot;%Msg&quot;}%n"/>
        <format id="syslog-common" format="%CustomSyslogHeader(20,{{.syslogRFC}}) {{.loggerName}} | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg%n" />
	</formats>
</seelog>`

func (c *SeelogConfig) setValue(k string, v interface{}) {
	c.m.Lock()
	defer c.m.Unlock()
	c.settings[k] = v
}

func (c *SeelogConfig) render() (string, error) {
	c.m.Lock()
	defer c.m.Unlock()
	funcMap := template.FuncMap{
		"ToLower": strings.ToLower,
	}

	tmpl, err := template.New("seelog_config").Funcs(funcMap).Parse(seelogConfigurationTemplate)
	if err != nil {
		return "", err
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, c.settings)
	return b.String(), err
}

func (c *SeelogConfig) enableConsoleLog(v bool) {
	c.setValue("consoleLoggingEnabled", v)
}

func (c *SeelogConfig) setLogLevel(l string) {
	c.setValue("logLevel", l)
}

func (c *SeelogConfig) enableFileLogging(f string, maxsize, maxrolls uint) {
	c.m.Lock()
	defer c.m.Unlock()
	c.settings["logfile"] = f
	c.settings["maxsize"] = maxsize
	c.settings["maxrolls"] = maxrolls
}

func (c *SeelogConfig) configureSyslog(syslogURI string, usetTLS bool) {
	c.m.Lock()
	defer c.m.Unlock()
	c.settings["syslogURI"] = syslogURI
	c.settings["syslogUseTLS"] = usetTLS

}

// NewSeelogConfig return a SeelogConfig filled with correct parameters
func NewSeelogConfig(name, level, format, jsonFormat, commonFormat string, syslogRFC bool) *SeelogConfig {
	c := &SeelogConfig{settings: make(map[string]interface{})}
	c.settings["loggerName"] = name
	c.settings["format"] = format
	c.settings["syslogRFC"] = syslogRFC
	c.settings["jsonFormat"] = jsonFormat
	c.settings["commonFormat"] = commonFormat
	c.settings["logLevel"] = level
	return c
}
