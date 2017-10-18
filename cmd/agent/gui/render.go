package gui

import (
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"html/template"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/status"
)

var fmap = template.FuncMap{
	"stringToHTML":       stringToHTML,
	"lastErrorTraceback": lastErrorTraceback,
	"lastErrorMessage":   status.LastErrorMessage,
	"pythonLoaderError":  pythonLoaderError,
	"formatUnixTime":     status.FormatUnixTime,
	"humanizeF":          status.MkHuman,
	"humanizeI":          mkHumanI,
	"formatTitle":        formatTitle,
	"add":                add,
	"instances":          instances,
}

// Data is a struct used for filling templates
type Data struct {
	Name       string
	LoaderErrs map[string]autodiscovery.LoaderErrors
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
	loaderErrs := autodiscovery.GetLoaderErrors()
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

	loaderErrs := autodiscovery.GetLoaderErrors()
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

func stringToHTML(value string) template.HTML {
	return template.HTML(value)
}

func pythonLoaderError(value string) template.HTML {
	value = strings.Replace(value, "', '", "", -1)
	value = strings.Replace(value, "['", "", -1)
	value = strings.Replace(value, "\\n']", "", -1)
	value = strings.Replace(value, "']", "", -1)
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

	lastErrorArray[0]["traceback"] = strings.Replace(lastErrorArray[0]["traceback"], "\n", "<br>", -1)
	lastErrorArray[0]["traceback"] = strings.Replace(lastErrorArray[0]["traceback"], "  ", "&nbsp;&nbsp;&nbsp;", -1)

	return template.HTML(lastErrorArray[0]["traceback"])
}

// same as status.mkHuman, but accepts integer input (vs float)
func mkHumanI(i int64) string {
	str := fmt.Sprintf("%d", i)

	if i > 1000000 {
		str = "over 1M"
	} else if i > 100000 {
		str = "over 100K"
	}

	return str
}

func formatTitle(title string) string {
	if title == "os" {
		return "OS"
	}

	// Split camel case words
	var words []string
	l := 0
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
