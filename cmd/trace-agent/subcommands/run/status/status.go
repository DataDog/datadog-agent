// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package status
package status

import (
	"bytes"
	"embed"
	"encoding/json"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	statusCore "github.com/DataDog/datadog-agent/comp/core/status"
)

type statusProvider struct {
	client ipc.HTTPClient
	url    string
}

func New(client ipc.HTTPClient, URL string) *statusProvider {
	return &statusProvider{
		client: client,
		url:    URL,
	}
}

//go:embed status_templates
var templatesFS embed.FS

func (s *statusProvider) Get(format string) string {
	status := map[string]interface{}{}

	resp, err := s.client.Get(s.url, ipchttp.WithCloseConnection)
	if err != nil {
		status["apmStats"] = map[string]string{
			"error": err.Error(),
		}
	} else {
		err = json.Unmarshal(resp, &status)
		if err != nil {
			status["apmStats"] = map[string]string{
				"error": err.Error(),
			}
		}
	}
	status = map[string]interface{}{
		"apmStats": status,
	}

	switch format {
	case "text":
		b := new(bytes.Buffer)
		err := statusCore.RenderText(templatesFS, "traceagent.tmpl", b, status)
		if err != nil {
			panic(err.Error())
		}

		return b.String()
	case "html":
		b := new(bytes.Buffer)
		err := statusCore.RenderHTML(templatesFS, "traceagentHTML.tmpl", b, status)
		if err != nil {
			panic(err.Error())
		}
		return b.String()
	case "json":
		jsonBytes, err := json.Marshal(status)
		if err != nil {
			panic(err.Error())
		}
		return string(jsonBytes)
	default:
		return "Unsupported format"
	}
}
