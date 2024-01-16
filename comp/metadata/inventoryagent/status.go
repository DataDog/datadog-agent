// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagent

import (
	"embed"
	"io"
	"path"

	htmlTemplate "html/template"
	textTemplate "text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

func (ia *inventoryagent) Name() string {
	return "metadata"
}

func (ia *inventoryagent) Index() int {
	return 3
}

func (ia *inventoryagent) JSON(verbose bool, stats map[string]interface{}) error {
	for k, v := range ia.Get() {
		stats[k] = v
	}

	return nil
}

func (ia *inventoryagent) Text(verbose bool, buffer io.Writer) error {
	return renderText(buffer, ia.Get())
}

func (ia *inventoryagent) HTML(verbose bool, buffer io.Writer) error {
	return renderHTML(buffer, ia.Get())
}

func renderHTML(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "inventoryHTML.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("inventoryHTML").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "inventory.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("inventory").Funcs(status.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
