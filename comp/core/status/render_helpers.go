// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"embed"
	"encoding/json"
	"fmt"
	htemplate "html/template"
	"io"
	"path"
	"strconv"
	"strings"
	"sync"
	ttemplate "text/template"
	"time"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/spf13/cast"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

var (
	htmlFuncOnce sync.Once
	htmlFuncMap  htemplate.FuncMap
	textFuncOnce sync.Once
	textFuncMap  ttemplate.FuncMap
)

// HTMLFmap return a map of utility functions for HTML templating
func HTMLFmap() htemplate.FuncMap {
	htmlFuncOnce.Do(func() {
		htmlFuncMap = htemplate.FuncMap{
			"doNotEscape":        doNotEscape,
			"lastError":          lastError,
			"configError":        configError,
			"printDashes":        PrintDashes,
			"formatUnixTime":     formatUnixTime,
			"humanize":           mkHuman,
			"humanizeDuration":   mkHumanDuration,
			"toUnsortedList":     toUnsortedList,
			"formatTitle":        formatTitle,
			"add":                add,
			"redText":            redText,
			"yellowText":         yellowText,
			"greenText":          greenText,
			"ntpWarning":         ntpWarning,
			"version":            getVersion,
			"percent":            func(v float64) string { return fmt.Sprintf("%02.1f", v*100) },
			"complianceResult":   complianceResult,
			"lastErrorTraceback": lastErrorTracebackHTML,
			"lastErrorMessage":   lastErrorMessageHTML,
			"pythonLoaderError":  pythonLoaderErrorHTML,
			"status":             statusHTML,
		}
	})
	return htmlFuncMap
}

// TextFmap map of utility functions for text templating
func TextFmap() ttemplate.FuncMap {
	textFuncOnce.Do(func() {
		textFuncMap = ttemplate.FuncMap{
			"lastErrorTraceback": lastErrorTraceback,
			"lastErrorMessage":   lastErrorMessage,
			"printDashes":        PrintDashes,
			"formatUnixTime":     formatUnixTime,
			"formatJSON":         formatJSON,
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
	})

	return textFuncMap
}

const timeFormat = "2006-01-02 15:04:05.999 MST"

// RenderHTML reads, parse and execute template from embed.FS
func RenderHTML(templateFS embed.FS, template string, buffer io.Writer, data any) error {
	tmpl, tmplErr := templateFS.ReadFile(path.Join("status_templates", template))
	if tmplErr != nil {
		return tmplErr
	}
	t := htemplate.Must(htemplate.New(template).Funcs(HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

// RenderText reads, parse and execute template from embed.FS
func RenderText(templateFS embed.FS, template string, buffer io.Writer, data any) error {
	tmpl, tmplErr := templateFS.ReadFile(path.Join("status_templates", template))
	if tmplErr != nil {
		return tmplErr
	}
	t := ttemplate.Must(ttemplate.New(template).Funcs(TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
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
func formatUnixTime(unixTime any) string {
	// Initially treat given unixTime is in nanoseconds
	parseFunction := func(value int64) string {
		t := time.Unix(0, value)
		// If year returned 1970, assume unixTime actually in seconds
		if t.Year() == time.Unix(0, 0).Year() {
			t = time.Unix(value, 0)
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

	switch v := unixTime.(type) {
	case int64:
		return parseFunction(v)
	case float64:
		return parseFunction(int64(v))
	default:
		return fmt.Sprintf("Invalid time parameter %T", v)
	}
}

// formatJSON formats the given value as JSON. The indent parameter is used to indent the entire JSON output.
func formatJSON(value interface{}, indent int) string {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error formatting JSON: %s", err)
	}
	return strings.Join(strings.Split(string(b), "\n"), "\n"+strings.Repeat(" ", indent))
}

// PrintDashes repeats the pattern (dash) for the length of s
func PrintDashes(s string, dash string) string {
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
func mkHuman(f interface{}) string {
	return humanize.Commaf(cast.ToFloat64(f))
}

// mkHumanDuration makes time values more readable
func mkHumanDuration(i interface{}, unit string) string {
	f := cast.ToFloat64(i)
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
	return cases.Title(language.English, cases.NoLower).String(title)
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

func pythonLoaderErrorHTML(value string) htemplate.HTML {
	value = htemplate.HTMLEscapeString(value)

	value = strings.Replace(value, "\n", "<br>", -1)
	value = strings.Replace(value, "  ", "&nbsp;&nbsp;&nbsp;", -1)
	return htemplate.HTML(value)
}

func lastErrorTracebackHTML(value string) htemplate.HTML {
	var lastErrorArray []map[string]string

	err := json.Unmarshal([]byte(value), &lastErrorArray)
	if err != nil || len(lastErrorArray) == 0 {
		return htemplate.HTML("No traceback")
	}

	traceback := htemplate.HTMLEscapeString(lastErrorArray[0]["traceback"])

	traceback = strings.Replace(traceback, "\n", "<br>", -1)
	traceback = strings.Replace(traceback, "  ", "&nbsp;&nbsp;&nbsp;", -1)

	return htemplate.HTML(traceback)
}

func lastErrorMessageHTML(value string) htemplate.HTML {
	var lastErrorArray []map[string]string
	err := json.Unmarshal([]byte(value), &lastErrorArray)
	if err == nil && len(lastErrorArray) > 0 {
		if msg, ok := lastErrorArray[0]["message"]; ok {
			value = msg
		}
	}
	return htemplate.HTML(htemplate.HTMLEscapeString(value))
}

func statusHTML(check map[string]interface{}) htemplate.HTML {
	if check["LastError"].(string) != "" {
		return htemplate.HTML("[<span class=\"error\">ERROR</span>]")
	}
	if len(check["LastWarnings"].([]interface{})) != 0 {
		return htemplate.HTML("[<span class=\"warning\">WARNING</span>]")
	}
	return htemplate.HTML("[<span class=\"ok\">OK</span>]")
}
