package gui

import (
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"html/template"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

var fmap = template.FuncMap{
	"lastErrorTraceback": lastErrorTraceback,
	"lastErrorMessage":   lastErrorMessage,
	"pythonLoaderError":  pythonLoaderError,
	"formatUnixTime":     formatUnixTime,
	"humanizeF":          mkHuman,
	"humanizeI":          mkHumanI,
	"formatTitle":        formatTitle,
	"add":                add,
	"instances":          instances,
}

const (
	timeFormat = "2006-01-02 15:04:05.000000 UTC"
)

// Data is a struct used for filling templates
type Data struct {
	Name       string
	LoaderErrs map[string]map[string]string
	ConfigErrs map[string]string
	Stats      map[string]interface{}
	CheckStats []*check.Stats
}

func renderStatus(rawData []byte, request string) (string, error) {
	var b = new(bytes.Buffer)
	stats := make(map[string]interface{})
	json.Unmarshal(rawData, &stats)

	data := Data{Stats: stats}
	e := fillTemplate(b, data, request+"Status")
	if e != nil {
		return "", e
	}
	return b.String(), nil
}

func renderRunningChecks() (string, error) {
	var b = new(bytes.Buffer)

	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats)
	loaderErrs := collector.GetLoaderErrors()
	configErrs := autodiscovery.GetConfigErrors()

	data := Data{LoaderErrs: loaderErrs, ConfigErrs: configErrs, Stats: runnerStats}
	e := fillTemplate(b, data, "runningChecks")
	if e != nil {
		return "", e
	}
	return b.String(), nil
}

func renderCheck(name string, stats []*check.Stats) (string, error) {
	var b = new(bytes.Buffer)

	data := Data{Name: name, CheckStats: stats}
	e := fillTemplate(b, data, "singleCheck")
	if e != nil {
		return "", e
	}
	return b.String(), nil
}

func renderError(name string) (string, error) {
	var b = new(bytes.Buffer)

	loaderErrs := collector.GetLoaderErrors()
	configErrs := autodiscovery.GetConfigErrors()

	data := Data{Name: name, LoaderErrs: loaderErrs, ConfigErrs: configErrs}
	e := fillTemplate(b, data, "loaderErr")
	if e != nil {
		return "", e
	}
	return b.String(), nil
}

func fillTemplate(w io.Writer, data Data, request string) error {
	t := template.New(request + ".tmpl")
	t.Funcs(fmap)
	t, e := t.ParseFiles(filepath.Join(common.GetViewsPath(), "templates/"+request+".tmpl"))
	if e != nil {
		return e
	}

	e = t.Execute(w, data)
	return e
}

/****** Helper functions for the template formatting ******/

func pythonLoaderError(value string) template.HTML {
	value = strings.Replace(value, "', '", "", -1)
	value = strings.Replace(value, "['", "", -1)
	value = strings.Replace(value, "\\n']", "", -1)
	value = strings.Replace(value, "']", "", -1)

	value = template.HTMLEscapeString(value)

	value = strings.Replace(value, "\\n", "<br>", -1)
	value = strings.Replace(value, "  ", "&nbsp;&nbsp;&nbsp;", -1)
	var loaderErrorArray []string
	json.Unmarshal([]byte(value), &loaderErrorArray)
	return template.HTML(value)
}

func lastErrorTraceback(value string) template.HTML {
	var lastErrorArray []map[string]string

	err := json.Unmarshal([]byte(value), &lastErrorArray)
	if err != nil || len(lastErrorArray) == 0 {
		return template.HTML("No traceback")
	}

	traceback := template.HTMLEscapeString(lastErrorArray[0]["traceback"])

	traceback = strings.Replace(traceback, "\n", "<br>", -1)
	traceback = strings.Replace(traceback, "  ", "&nbsp;&nbsp;&nbsp;", -1)

	return template.HTML(traceback)
}

func mkHumanI(i int64) string {
	return mkHuman(float64(i))
}

func mkHuman(f float64) string {
	if f > 1000000 {
		return fmt.Sprintf("%.1fM", f/1000000.0)
	} else if f > 100000 {
		return fmt.Sprintf("%.1fK", f/1000.0)
	}
	return fmt.Sprintf("%.0f", f)
}

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

func lastErrorMessage(value string) string {
	var lastErrorArray []map[string]string
	err := json.Unmarshal([]byte(value), &lastErrorArray)
	if err == nil && len(lastErrorArray) > 0 {
		if msg, ok := lastErrorArray[0]["message"]; ok {
			return msg
		}
	}
	return "UNKNOWN ERROR"
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

func add(x, y int) int {
	return x + y
}

func instances(checks map[string]interface{}) map[string][]interface{} {
	instances := make(map[string][]interface{})
	for _, ch := range checks {
		if check, ok := ch.(map[string]interface{}); ok {
			if name, ok := check["CheckName"].(string); ok {
				if len(instances[name]) == 0 {
					instances[name] = []interface{}{check}
				} else {
					instances[name] = append(instances[name], check)
				}
			}
		}
	}
	return instances
}
