// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpproxy

import (
	"embed"
	htmlTemplate "html/template"
	"io"
	"path"
	textTemplate "text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

//go:embed status_templates
var templatesFS embed.FS

type Provider struct{}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}

func (p Provider) populateStatus(stats map[string]interface{}) {
	stats["TransportWarnings"] = httputils.GetNumberOfWarnings() > 0
	stats["NoProxyIgnoredWarningMap"] = httputils.GetProxyIgnoredWarnings()
	stats["NoProxyUsedInFuture"] = httputils.GetProxyUsedInFutureWarnings()
	stats["NoProxyChanged"] = httputils.GetProxyIgnoredWarnings()
}

func (p Provider) Name() string {
	return "Transport Proxy Warnings"
}

func (p Provider) Section() string {
	return "Transport Proxy Warnings"
}

func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

func (p Provider) Text(_ bool, buffer io.Writer) error {
	return renderText(buffer, p.getStatusInfo())
}

func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return renderHTML(buffer, p.getStatusInfo())
}

func renderHTML(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "httpHTML.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("httpHTML").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "http.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("http").Funcs(status.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
