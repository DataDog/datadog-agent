// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

var (
	here, _        = executable.Folder()
	fmap           template.FuncMap // helper functions available in templates
	templateFolder string
)

func init() {
	templateFolder = filepath.Join(common.GetDistPath(), "templates")
}

// SetTemplateFolder overrides the template folder used to render templates.
// It is should only be used for tests.
func SetTemplateFolder(folder string) {
	templateFolder = folder
}

// FormatTemplate renders the specified go template using the json data.
func FormatTemplate(data []byte, tmplFile string) (string, error) {
	buf := new(bytes.Buffer)
	status := make(map[string]interface{})
	err := json.Unmarshal(data, &status)
	if err != nil {
		return "", err
	}
	Template(buf, tmplFile, status)
	return buf.String(), nil
}

// Template renders the specified go template with the status info.
func Template(w io.Writer, tmplFile string, status interface{}) {
	t := template.Must(template.New(tmplFile).Funcs(fmap).ParseFiles(filepath.Join(templateFolder, tmplFile)))
	err := t.Execute(w, status)
	if err != nil {
		fmt.Println(err)
	}
}

// ChecksStats renders the collector template with the check stats info.
func ChecksStats(w io.Writer, runnerStats, pyLoaderStats, autoConfigStats, checkSchedulerStats interface{}, onlyCheck string) {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["pyLoaderStats"] = pyLoaderStats
	checkStats["AutoConfigStats"] = autoConfigStats
	checkStats["CheckSchedulerStats"] = checkSchedulerStats
	checkStats["OnlyCheck"] = onlyCheck
	Template(w, "collector.tmpl", checkStats)
}
