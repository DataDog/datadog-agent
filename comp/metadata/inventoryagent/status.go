// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagent

import (
	"embed"
	"io"
	"path"

	"github.com/DataDog/datadog-agent/comp/core/status"

	htmlTemplate "html/template"
	textTemplate "text/template"
)

type statusProvider struct {
	data agentMetadata
}

func (s statusProvider) Section() status.StatusSection {
	return status.MetadataSection
}

func (s statusProvider) Name() string {
	return "Metadata"
}

func (s statusProvider) JSON(stats map[string]interface{}) {
	data := map[string]interface{}{}
	for k, v := range s.data {
		data[k] = v
	}
	stats["agent_metadata"] = data
}

//go:embed templates
var templatesFS embed.FS

func (s statusProvider) Text(buffer io.Writer) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "text.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("metadata").Parse(string(tmpl)))
	return t.Execute(buffer, s.data)
}

func (s statusProvider) HTML(buffer io.Writer) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "html.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("metadata").Parse(string(tmpl)))
	return t.Execute(buffer, s.data)
}

func (s statusProvider) AppendToHeader(stats map[string]interface{}) {}

// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
func (ia *inventoryagent) statusProvider() status.Provider {
	return status.NewProvider(statusProvider{
		data: ia.data,
	})
}
