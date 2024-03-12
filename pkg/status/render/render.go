// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package render has all the formating options for status output
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

var fmap = status.TextFmap()

// FormatHPAStatus takes a json bytestring and prints out the formatted statuspage
func FormatHPAStatus(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	var b = new(bytes.Buffer)
	var errs []error
	if err := ParseTemplate(b, "/custommetricsprovider.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		fmt.Println(err)
	}
	return b.String(), nil
}

// FormatMetadataMapCLI builds the rendering in the metadataMapper template.
func FormatMetadataMapCLI(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	var b = new(bytes.Buffer)
	var errs []error
	if err := ParseTemplate(b, "/metadatamapper.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		return "", err
	}
	return b.String(), nil
}

//go:embed templates
var templatesFS embed.FS

// ParseTemplate renders the text template with the data provided
func ParseTemplate(w io.Writer, templateName string, stats interface{}) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", templateName))
	if tmplErr != nil {
		return tmplErr
	}
	t, err := template.New(templateName).Funcs(fmap).Parse(string(tmpl))
	if err != nil {
		return err
	}
	return t.Execute(w, stats)
}

func renderErrors(w io.Writer, errs []error) error {
	if len(errs) > 0 {
		return ParseTemplate(w, "/rendererrors.tmpl", errs)
	}
	return nil
}

func unmarshalStatus(data []byte) (stats map[string]interface{}, renderError string, err error) {
	if err := json.Unmarshal(data, &stats); err != nil {
		var b = new(bytes.Buffer)
		if err := renderErrors(b, []error{err}); err != nil {
			return nil, "", err
		}
		return nil, b.String(), nil
	}
	return stats, "", nil
}
