// +build serverless

package config

import (
	"fmt"
	"strings"

	"github.com/cihub/seelog"
)

// buildCommonFormat returns the log common format seelog string
func buildCommonFormat(loggerName LoggerName) string {
	return fmt.Sprintf("%%Date(%s) | %s | %%LEVEL | %%Msg%%n", getLogDateFormat(), loggerName)
}

// buildJSONFormat returns the log JSON format seelog string
func buildJSONFormat(loggerName LoggerName) string {
	seelog.RegisterCustomFormatter("QuoteMsg", createQuoteMsgFormatter) //nolint:errcheck
	return fmt.Sprintf(`{"agent":"%s","time":"%%Date(%s)","level":"%%LEVEL","file":"","line":"","func":"%%FuncShort","msg":%%QuoteMsg}%%n`, strings.ToLower(string(loggerName)), getLogDateFormat())
}
