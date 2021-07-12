// +build !serverless

package config

import (
	"fmt"
	"strings"

	"github.com/cihub/seelog"
)

// buildCommonFormat returns the log common format seelog string
func buildCommonFormat(loggerName LoggerName) string {
	if loggerName == "JMXFETCH" {
		return `%Msg%n`
	}
	return fmt.Sprintf("%%Date(%s) | %s | %%LEVEL | (%%ShortFilePath:%%Line in %%FuncShort) | %%ExtraTextContext%%Msg%%n", getLogDateFormat(), loggerName)
}

// buildJSONFormat returns the log JSON format seelog string
func buildJSONFormat(loggerName LoggerName) string {
	seelog.RegisterCustomFormatter("QuoteMsg", createQuoteMsgFormatter) //nolint:errcheck
	if loggerName == "JMXFETCH" {
		return `{"msg":%QuoteMsg}%n`
	}
	return fmt.Sprintf(`{"agent":"%s","time":"%%Date(%s)","level":"%%LEVEL","file":"%%ShortFilePath","line":"%%Line","func":"%%FuncShort","msg":%%QuoteMsg%%ExtraJSONContext}%%n`, strings.ToLower(string(loggerName)), getLogDateFormat())
}
