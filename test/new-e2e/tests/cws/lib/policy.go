// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2elib

import (
	"bytes"
	"text/template"
)

// TestRuleDefinition defines a rule used in a test policy
type TestRuleDefinition struct {
	ID         string
	Version    string
	Expression string
}

const testPolicyTemplate = `---
version: 1.2.3

rules:
{{range $Rule := .Rules}}
  - id: {{$Rule.ID}}
    version: {{$Rule.Version}}
    expression: >-
      {{$Rule.Expression}}
{{end}}
`

// GetPolicyContent returns the policy content from the given rules and macros definitions
func GetPolicyContent(rules []*TestRuleDefinition) (string, error) {
	tmpl, err := template.New("policy").Parse(testPolicyTemplate)
	if err != nil {
		return "", err
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"Rules": rules,
	}); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
