// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagent

import (
	"embed"
	htmlTemplate "html/template"
	"io"
	"path"
	textTemplate "text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

type statusProvider struct {
	ia *inventoryagent
}

func newStatusProvider(ia *inventoryagent) statusProvider {
	return statusProvider{
		ia: ia,
	}
}

func (s statusProvider) Name() string {
	return "metadata"
}

func (s statusProvider) Index() int {
	return 1
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	return s.ia.Get()
}

func (s statusProvider) JSON(stats map[string]interface{}) error {
	for k, v := range s.getStatusInfo() {
		stats[k] = v
	}

	return nil
}

func (s statusProvider) Text(buffer io.Writer) error {
	return renderText(buffer, s.getStatusInfo())
}

func (s statusProvider) HTML(buffer io.Writer) error {
	return renderHTML(buffer, s.getStatusInfo())
}

//go:embed status_templates
var templatesFS embed.FS

func renderHTML(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "statusHTML.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("inventoryHTML").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "status.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("inventory").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
