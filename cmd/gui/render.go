package gui

import (
	"bytes"
	"encoding/json"
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
	"humanize":           status.MkHuman,
	"humanizeInt":        mkHuman,
	"formatTitle":        formatTitle,
	"add":                add,
}

// CheckStats is the struct used for filling templates/singleCheck.tmpl
type CheckStats struct {
	Name  string
	Stats []*check.Stats
}

// Errors is the struct used for filling templates/loaderErr.tmpl
type Errors struct {
	Name       string
	LoaderErrs map[string]autodiscovery.LoaderErrors
	ConfigErrs map[string]string
}

func renderStatus(data []byte, request string) (string, error) {
	var b = new(bytes.Buffer)
	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats)

	e := fillTemplate(b, stats, request+"Status")
	if e != nil {
		return "", e
	}
	return b.String(), nil
}

func fillTemplate(w io.Writer, stats map[string]interface{}, request string) error {
	t := template.New(request + ".tmpl")
	t.Funcs(fmap)
	t, e := t.ParseFiles(filepath.Join(common.GetViewPath(), "templates/"+request+".tmpl"))
	if e != nil {
		return e
	}

	e = t.Execute(w, stats)
	return e
}

func renderCheck(name string, stats []*check.Stats) (string, error) {
	var b = new(bytes.Buffer)

	t := template.New("singleCheck.tmpl")
	t.Funcs(fmap)
	t, e := t.ParseFiles(filepath.Join(common.GetViewPath(), "templates/singleCheck.tmpl"))
	if e != nil {
		return "", e
	}

	cs := CheckStats{name, stats}
	e = t.Execute(b, cs)
	if e != nil {
		return "", e
	}
	return b.String(), nil
}

func renderError(name string) (string, error) {
	var b = new(bytes.Buffer)

	t := template.New("loaderErr.tmpl")
	t.Funcs(fmap)
	t, e := t.ParseFiles(filepath.Join(common.GetViewPath(), "templates/loaderErr.tmpl"))
	if e != nil {
		return "", e
	}

	loaderErrs := autodiscovery.GetLoaderErrors()
	configErrs := autodiscovery.GetConfigErrors()

	errs := Errors{name, loaderErrs, configErrs}
	e = t.Execute(b, errs)
	if e != nil {
		return "", e
	}
	return b.String(), nil
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

func mkHuman(i int64) string {
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
