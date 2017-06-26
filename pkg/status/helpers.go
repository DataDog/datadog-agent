package status

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

func init() {
	fmap = template.FuncMap{
		"doNotEscape":        doNotEscape,
		"lastError":          lastError,
		"lastErrorTraceback": lastErrorTraceback,
		"lastErrorMessage":   lastErrorMessage,
		"pythonLoaderError":  pythonLoaderError,
		"printDashes":        printDashes,
		"formatUnixTime":     formatUnixTime,
	}
}

func doNotEscape(value string) template.HTML {
	return template.HTML(value)
}

func pythonLoaderError(value string) template.HTML {
	value = strings.Replace(value, "', '", "", -1)
	value = strings.Replace(value, "['", "", -1)
	value = strings.Replace(value, "\\n']", "", -1)
	value = strings.Replace(value, "']", "", -1)
	value = strings.Replace(value, "\\n", "\n      ", -1)
	value = strings.TrimRight(value, "\n\t ")
	var loaderErrorArray []string
	json.Unmarshal([]byte(value), &loaderErrorArray)
	return template.HTML(value)
}

func lastError(value string) template.HTML {
	return template.HTML(value)
}

func lastErrorTraceback(value string) template.HTML {
	var lastErrorArray []map[string]string

	json.Unmarshal([]byte(value), &lastErrorArray)
	lastErrorArray[0]["traceback"] = strings.Replace(lastErrorArray[0]["traceback"], "\n", "\n      ", -1)
	lastErrorArray[0]["traceback"] = strings.TrimRight(lastErrorArray[0]["traceback"], "\n\t ")
	return template.HTML(lastErrorArray[0]["traceback"])
}

func lastErrorMessage(value string) template.HTML {
	var lastErrorArray []map[string]string

	json.Unmarshal([]byte(value), &lastErrorArray)
	return template.HTML(lastErrorArray[0]["message"])
}

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
	var dashes string
	for i := 0; i < stringLength(s); i++ {
		dashes += dash
	}
	return dashes
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
