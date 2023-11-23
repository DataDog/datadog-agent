// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2elib

import (
	"bytes"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const policyTemplate = `---
version: 1.2.3

macros:
{{range $Macro := .Macros}}
  - id: {{$Macro.ID}}
    expression: >-
      {{$Macro.Expression}}
{{end}}

rules:
{{range $Rule := .Rules}}
  - id: {{$Rule.ID}}
    version: {{$Rule.Version}}
    expression: >-
      {{$Rule.Expression}}
    tags:
{{- range $Tag, $Val := .Tags}}
      {{$Tag}}: {{$Val}}
{{- end}}
    actions:
{{- range $Action := .Actions}}
{{- if $Action.Set}}
      - set:
          name: {{$Action.Set.Name}}
		  {{- if $Action.Set.Value}}
          value: {{$Action.Set.Value}}
          {{- else if $Action.Set.Field}}
          field: {{$Action.Set.Field}}
          {{- end}}
          scope: {{$Action.Set.Scope}}
          append: {{$Action.Set.Append}}
{{- end}}
{{- if $Action.Kill}}
      - kill:
          {{- if $Action.Kill.Signal}}
          signal: {{$Action.Kill.Signal}}
          {{- end}}
{{- end}}
{{- end}}
{{end}}
`

// GetPolicyContent returns the policy content from the given rules and macros definitions
func GetPolicyContent(macros []*rules.MacroDefinition, rules []*rules.RuleDefinition) (string, error) {
	tmpl, err := template.New("policy").Parse(policyTemplate)
	if err != nil {
		return "", err
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"Rules":  rules,
		"Macros": macros,
	}); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
