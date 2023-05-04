// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	"bytes"
	"fmt"
	"text/template"
)

// SeclEncodingTemplate is the template used to generate profiles
var SeclEncodingTemplate = `---
name: {{ .Name }}
selector:
  - {{ .Selector }}

rules:{{ range .Rules }}
  - id: {{ .ID }}
    expression: {{ .Expression }}
{{ end }}
`

// EncodeSecL encodes an activity dump in the SecL format
func (ad *ActivityDump) EncodeSecL() (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	// generate selector
	var selector string
	if len(ad.Metadata.Comm) > 0 {
		selector = fmt.Sprintf("process.comm = \"%s\"", ad.Metadata.Comm)
	}

	t := template.Must(template.New("tmpl").Parse(SeclEncodingTemplate))
	raw := bytes.NewBuffer(nil)
	if err := t.Execute(raw, ad.ActivityTree.GenerateProfileData(selector)); err != nil {
		return nil, fmt.Errorf("couldn't generate profile: %w", err)
	}
	return raw, nil
}
