// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package status

import (
	"fmt"
	"github.com/segmentio/encoding/json"
	"html/template"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"

	"golang.org/x/text/unicode/norm"
)

// Fmap return a fresh copy of a map of utility functions for templating
func Fmap() template.FuncMap {
	return template.FuncMap{
		"doNotEscape":        doNotEscape,
		"lastError":          lastError,
		"lastErrorTraceback": lastErrorTraceback,
		"lastErrorMessage":   lastErrorMessage,
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
	}
}

func doNotEscape(value string) template.HTML {
	return template.HTML(value)
}

func configError(value string) template.HTML {
	return template.HTML(value + "\n")
}

func lastError(value string) template.HTML {
	return template.HTML(value)
}

func lastErrorTraceback(value string) template.HTML {
	var lastErrorArray []map[string]string

	err := json.Unmarshal([]byte(value), &lastErrorArray)
	if err != nil || len(lastErrorArray) == 0 {
		return template.HTML("No traceback")
	}
	lastErrorArray[0]["traceback"] = strings.Replace(lastErrorArray[0]["traceback"], "\n", "\n      ", -1)
	lastErrorArray[0]["traceback"] = strings.TrimRight(lastErrorArray[0]["traceback"], "\n\t ")
	return template.HTML(lastErrorArray[0]["traceback"])
}

// lastErrorMessage converts the last error message to html
func lastErrorMessage(value string) template.HTML {
	var lastErrorArray []map[string]string
	err := json.Unmarshal([]byte(value), &lastErrorArray)
	if err == nil && len(lastErrorArray) > 0 {
		if msg, ok := lastErrorArray[0]["message"]; ok {
			return template.HTML(msg)
		}
	}
	return template.HTML(value)
}

// formatUnixTime formats the unix time to make it more readable
func formatUnixTime(unixTime float64) string {
	var (
		sec  int64
		nsec int64
	)
	ts := fmt.Sprintf("%f", unixTime)
	secs := strings.Split(ts, ".")
	sec, _ = strconv.ParseInt(secs[0], 10, 64)
	if len(secs) == 2 {
		nsec, _ = strconv.ParseInt(secs[1], 10, 64)
	}
	t := time.Unix(sec, nsec)
	return t.Format(timeFormat)
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

// mkHuman makes large numbers more readable
func mkHuman(f float64) string {
	var str string
	if f > 1000000.0 {
		str = humanize.SIWithDigits(f, 1, "")
	} else {
		str = humanize.Commaf(f)
	}

	return str
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
