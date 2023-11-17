// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	htmlTemplate "html/template"
	"io"
	"path"
	textTemplate "text/template"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type template interface {
	Execute(wr io.Writer, data any) error
}

var templates = map[string]template{
	"status.text": parseTextTemplate("templates/text.tmpl"),
	"gui.html":    parseHTMLTemplate("templates/html.html"),
}

//go:embed templates
var templatesFS embed.FS

func parseTextTemplate(template string) template {
	tmpl, _ := templatesFS.ReadFile(path.Join("templates", template))
	return textTemplate.Must(textTemplate.New(template).Parse(string(tmpl)))
}

func parseHTMLTemplate(template string) template {
	tmpl, _ := templatesFS.ReadFile(path.Join("templates", template))
	return htmlTemplate.Must(htmlTemplate.New(template).Parse(string(tmpl)))
}

type dependencies struct {
	fx.In

	Providers []status.StatusProvider `group:"status"`
}

type statusImplementation struct {
	providers []status.StatusProvider
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newStatus),
)

func newStatus(deps dependencies) (status.Component, error) {
	return &statusImplementation{
		providers: deps.Providers,
	}, nil
}

func (s *statusImplementation) Get() ([]byte, error) {
	stats := make(map[string]interface{})
	for _, sc := range s.providers {
		sc(stats)
	}
	return json.Marshal(stats)
}

func (s *statusImplementation) Format(template string) ([]byte, error) {
	val, ok := templates[template]
	if !ok {
		return []byte{}, fmt.Errorf("template %s not found", template)
	}
	buffer := new(bytes.Buffer)
	stats := make(map[string]interface{})
	for _, sc := range s.providers {
		sc(stats)
	}
	err := val.Execute(buffer, stats)
	return buffer.Bytes(), err
}
