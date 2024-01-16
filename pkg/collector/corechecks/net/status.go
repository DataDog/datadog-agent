// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package net

import (
	"embed"
	"expvar"
	"io"
	"path"
	"strconv"

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
	return "Clocks"
}

// Index return the index
func (Provider) Index() int {
	return 2
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}

func (Provider) populateStatus(stats map[string]interface{}) {
	ntpOffset := expvar.Get("ntpOffset")
	if ntpOffset != nil && ntpOffset.String() != "" {
		float, err := strconv.ParseFloat(expvar.Get("ntpOffset").String(), 64)
		if err == nil {
			stats["ntpOffset"] = float
		}
	}
}

// JSON populates the status map
func (p Provider) JSON(verbose bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

func (p Provider) Text(verbose bool, buffer io.Writer) error {
	return renderText(buffer, p.getStatusInfo())
}

func (p Provider) HTML(verbose bool, buffer io.Writer) error {
	return renderHTML(buffer, p.getStatusInfo())
}

func renderHTML(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "ntpHTML.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("ntpHTML").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "ntp.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("ntp").Funcs(status.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
