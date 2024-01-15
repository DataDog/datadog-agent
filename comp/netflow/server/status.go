// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package server

import (
	"embed"
	"encoding/json"
	"io"
	"path"

	htmlTemplate "html/template"
	textTemplate "text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output with the collector information
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "NetFlow"
}

// Section return the section
func (Provider) Section() string {
	return "NetFlow"
}

// JSON populates the status map
func (Provider) JSON(stats map[string]interface{}) error {
	status := GetStatus()

	var statusMap map[string]interface{}
	statusBytes, err := json.Marshal(status)

	if err != nil {
		return err
	}

	err = json.Unmarshal(statusBytes, &statusMap)

	if err != nil {
		return err
	}

	for key, value := range statusMap {
		stats[key] = value
	}

	return nil
}

func (Provider) Text(buffer io.Writer) error {
	return renderText(buffer, GetStatus())
}

func (Provider) HTML(buffer io.Writer) error {
	return renderHTML(buffer, GetStatus())
}

func renderHTML(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "netflowHTML.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("netflowHTML").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "netflow.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("netflow").Funcs(status.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
