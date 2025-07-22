// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import "time"

const logDateFormat = "2006-01-02 15:04:05 MST" // see time.Format for format syntax

// Date returns a function that formats a time.Time to a string.
func Date(logFormatRFC3339 bool) func(time.Time) string {
	format := getLogDateFormat(logFormatRFC3339)
	return func(t time.Time) string {
		return t.Format(format)
	}
}

func getLogDateFormat(logFormatRFC3339 bool) string {
	if logFormatRFC3339 {
		return time.RFC3339
	}
	return logDateFormat
}
