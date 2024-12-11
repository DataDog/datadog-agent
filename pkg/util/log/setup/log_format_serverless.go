// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package logs

import (
	"fmt"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/cihub/seelog"
)

// buildCommonFormat returns the log common format seelog string
func buildCommonFormat(loggerName LoggerName, cfg pkgconfigmodel.Reader) string {
	return fmt.Sprintf("%%Date(%s) | %s | %%LEVEL | %%Msg%%n", getLogDateFormat(cfg), loggerName)
}

// buildJSONFormat returns the log JSON format seelog string
func buildJSONFormat(loggerName LoggerName, cfg pkgconfigmodel.Reader) string {
	seelog.RegisterCustomFormatter("QuoteMsg", createQuoteMsgFormatter) //nolint:errcheck
	return fmt.Sprintf(`{"agent":"%s","time":"%%Date(%s)","level":"%%LEVEL","file":"","line":"","func":"%%FuncShort","msg":%%QuoteMsg}%%n`, strings.ToLower(string(loggerName)), getLogDateFormat(cfg))
}
