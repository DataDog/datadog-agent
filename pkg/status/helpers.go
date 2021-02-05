// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package status

import (
	"encoding/json"
	"fmt"
	htemplate "html/template"
	"strconv"
	"strings"
	ttemplate "text/template"
	"time"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"

	"golang.org/x/text/unicode/norm"
)

// Fmap return a fresh copy of a map of utility functions for HTML templating
func Fmap() htemplate.FuncMap {
	return htemplate.FuncMap{
		"doNotEscape":        doNotEscape,
		"lastError":          lastError,
		"lastErrorTraceback": func(s string) htemplate.HTML { return doNotEscape(lastErrorTraceback(s)) },
		"lastErrorMessage":   func(s string) htemplate.HTML { return doNotEscape(lastErrorMessage(s)) },
		"configError":        configError,
		"printDashes":        printDashes,
		"formatUnixTime":     formatUnixTime,
		"humanize":           mkHuman,
		"humanizeDuration":   mkHumanDuration,
		"toUnsortedList":     toUnsortedList,
		"formatTitle":        formatTitle,
		"add":                add,
		"status":             status,
		"redText":            redText,
		"yellowText":         yellowText,
		"greenText":          greenText,
		"ntpWarning":         ntpWarning,
		"version":            getVersion,
		"percent":            func(v float64) string { return fmt.Sprintf("%02.1f", v*100) },
		"complianceResult":   complianceResult,
	}
}

// Textfmap return a fresh copy of a map of utility functions for text templating
func Textfmap() ttemplate.FuncMap {
	return ttemplate.FuncMap{
		"lastErrorTraceback": lastErrorTraceback,
		"lastErrorMessage":   lastErrorMessage,
		"printDashes":        printDashes,
		"formatUnixTime":     formatUnixTime,
		"humanize":           mkHuman,
		"humanizeDuration":   mkHumanDuration,
		"toUnsortedList":     toUnsortedList,
		"formatTitle":        formatTitle,
		"add":                add,
		"status":             status,
		"redText":            redText,
		"yellowText":         yellowText,
		"greenText":          greenText,
		"ntpWarning":         ntpWarning,
		"version":            getVersion,
		"percent":            func(v float64) string { return fmt.Sprintf("%02.1f", v*100) },
		"complianceResult":   complianceResult,
	}
}

func doNotEscape(value string) htemplate.HTML {
	return htemplate.HTML(value)
}

func configError(value string) htemplate.HTML {
	return htemplate.HTML(value + "\n")
}

func lastError(value string) htemplate.HTML {
	return htemplate.HTML(value)
}

func lastErrorTraceback(value string) string {
	var lastErrorArray []map[string]string

	err := json.Unmarshal([]byte(value), &lastErrorArray)
	if err != nil || len(lastErrorArray) == 0 {
		return "No traceback"
	}
	lastErrorArray[0]["traceback"] = strings.Replace(lastErrorArray[0]["traceback"], "\n", "\n      ", -1)
	lastErrorArray[0]["traceback"] = strings.TrimRight(lastErrorArray[0]["traceback"], "\n\t ")
	return lastErrorArray[0]["traceback"]
}

// lastErrorMessage converts the last error message to html
func lastErrorMessage(value string) string {
	var lastErrorArray []map[string]string
	err := json.Unmarshal([]byte(value), &lastErrorArray)
	if err == nil && len(lastErrorArray) > 0 {
		if msg, ok := lastErrorArray[0]["message"]; ok {
			return msg
		}
	}
	return value
}

// formatUnixTime formats the unix time to make it more readable
func formatUnixTime(unixTime float64) string {
	// Initially treat given unixTime is in nanoseconds
	t := time.Unix(0, int64(unixTime))
	// If year returned 1970, assume unixTime actually in seconds
	if t.Year() == time.Unix(0, 0).Year() {
		t = time.Unix(int64(unixTime), 0)
	}

	_, tzoffset := t.Zone()
	result := t.Format(timeFormat)
	if tzoffset != 0 {
		result += " / " + t.UTC().Format(timeFormat)
	}
	msec := t.UnixNano() / int64(time.Millisecond)
	result += " (" + strconv.Itoa(int(msec)) + ")"
	return result
}

func printDashes(s string, dash string) string {
	return strings.Repeat(dash, stringLength(s))
}

func toUnsortedList(s map[string]interface{}) string {
	res := make([]string, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	return fmt.Sprintf("%s", res)
}

// mkHuman adds commas to large numbers to assist readability in status outputs
func mkHuman(f float64) string {
	return humanize.Commaf(f)
}

// mkHumanDuration makes time values more readable
func mkHumanDuration(f float64, unit string) string {
	var duration time.Duration
	if unit != "" {
		duration, _ = time.ParseDuration(fmt.Sprintf("%f%s", f, unit))
	} else {
		duration = time.Duration(int64(f)) * time.Second
	}

	return duration.String()
}

func stringLength(s string) int {
	/*
		len(string) is wrong if the string has unicode characters in it,
		for example, something like 'Agent (v6.0.0+Χελωνη)' has len(s) == 27.
		This is a better way of counting a string length
		(credit goes to https://stackoverflow.com/a/12668840)
	*/
	var ia norm.Iter
	ia.InitString(norm.NFKD, s)
	nc := 0
	for !ia.Done() {
		nc = nc + 1
		ia.Next()
	}
	return nc
}

// add two integer together
func add(x, y int) int {
	return x + y
}

// formatTitle split a camel case string into space-separated words
func formatTitle(title string) string {
	if title == "os" {
		return "OS"
	}

	// Split camel case words
	var words []string
	var l int

	for s := title; s != ""; s = s[l:] {
		l = strings.IndexFunc(s[1:], unicode.IsUpper) + 1
		if l <= 0 {
			l = len(s)
		}
		words = append(words, s[:l])
	}
	title = strings.Join(words, " ")

	// Capitalize the first letter
	return strings.Title(title)
}

func status(check map[string]interface{}) string {
	if check["LastError"].(string) != "" {
		return fmt.Sprintf("[%s]", color.RedString("ERROR"))
	}
	if len(check["LastWarnings"].([]interface{})) != 0 {
		return fmt.Sprintf("[%s]", color.YellowString("WARNING"))
	}
	return fmt.Sprintf("[%s]", color.GreenString("OK"))
}

func complianceResult(result string) string {
	switch result {
	case "error":
		return fmt.Sprintf("[%s]", color.RedString("ERROR"))
	case "failed":
		return fmt.Sprintf("[%s]", color.RedString("FAILED"))
	case "passed":
		return fmt.Sprintf("[%s]", color.GreenString("PASSED"))
	default:
		return fmt.Sprintf("[%s]", color.YellowString("UNKNOWN"))
	}
}

// Renders the message in a red color
func redText(message string) string {
	return color.RedString(message)
}

// Renders the message in a yellow color
func yellowText(message string) string {
	return color.YellowString(message)
}

// Renders the message in a green color
func greenText(message string) string {
	return color.GreenString(message)
}

// Tells if the ntp offset may be too large, resulting in metrics
// from the agent being dropped by metrics-intake
func ntpWarning(ntpOffset float64) bool {
	// Negative offset => clock is in the future, 10 minutes (600s) allowed
	// Positive offset => clock is in the past, 60 minutes (3600s) allowed
	// According to https://docs.datadoghq.com/developers/metrics/#submitting-metrics
	return ntpOffset <= -600 || ntpOffset >= 3600
}

func getVersion(instances map[string]interface{}) string {
	if len(instances) == 0 {
		return ""
	}
	for _, instance := range instances {
		instanceMap := instance.(map[string]interface{})
		version, ok := instanceMap["CheckVersion"]
		if !ok {
			return ""
		}
		str, ok := version.(string)
		if !ok {
			return ""
		}
		return str
	}
	return ""
}
