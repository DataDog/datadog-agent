// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package main implements secl-check, a CLI tool that validates SECL expressions
// and provides field metadata for agentic pipelines.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Diagnostic represents a single validation finding.
type Diagnostic struct {
	Severity    string   `json:"severity"`
	Message     string   `json:"message"`
	Column      int      `json:"column,omitempty"`
	Field       string   `json:"field,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// FieldInfo describes a SECL field.
type FieldInfo struct {
	Name      string `json:"name"`
	EventType string `json:"event_type,omitempty"`
	Type      string `json:"type"`
	IsArray   bool   `json:"is_array,omitempty"`
}

// CheckResult is the output of expression validation.
type CheckResult struct {
	Valid       bool         `json:"valid"`
	Expression  string       `json:"expression"`
	EventType   string       `json:"event_type,omitempty"`
	Fields      []string     `json:"fields,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// ListResult is the output of field/event listing.
type ListResult struct {
	EventTypes []string    `json:"event_types,omitempty"`
	Fields     []FieldInfo `json:"fields,omitempty"`
}

func main() {
	exprFlag := flag.String("expr", "", "SECL expression to validate")
	listFieldsFlag := flag.Bool("list-fields", false, "List all available fields")
	eventTypeFlag := flag.String("event-type", "", "Filter fields by event type")
	suggestFlag := flag.String("suggest", "", "Suggest fields matching a partial name")
	listEventsFlag := flag.Bool("list-events", false, "List all event types")
	listConstantsFlag := flag.Bool("list-constants", false, "List all available constants")
	prettyFlag := flag.Bool("pretty", false, "Pretty-print JSON output")
	flag.Parse()

	var result interface{}

	switch {
	case *exprFlag != "":
		result = validateExpression(*exprFlag)
	case *listFieldsFlag:
		result = listFields(*eventTypeFlag)
	case *suggestFlag != "":
		result = suggestFieldsCmd(*suggestFlag, *eventTypeFlag)
	case *listEventsFlag:
		result = listEventTypes()
	case *listConstantsFlag:
		result = serializableConstants()
	default:
		flag.Usage()
		os.Exit(1)
	}

	var out []byte
	if *prettyFlag {
		out, _ = json.MarshalIndent(result, "", "  ")
	} else {
		out, _ = json.Marshal(result)
	}
	fmt.Println(string(out))

	if cr, ok := result.(*CheckResult); ok && !cr.Valid {
		os.Exit(1)
	}
}

// newModel creates a model configured with legacy fields.
func newModel() *model.Model {
	m := &model.Model{}
	m.SetLegacyFields(model.SECLLegacyFields)
	return m
}

// newEvalOpts returns eval options configured with constants, legacy fields, and variables.
func newEvalOpts() *eval.Opts {
	var opts eval.Opts
	opts.
		WithConstants(model.SECLConstants()).
		WithLegacyFields(model.SECLLegacyFields).
		WithVariables(model.SECLVariables)
	return &opts
}

func validateExpression(expr string) *CheckResult {
	result := &CheckResult{
		Expression: expr,
	}

	parsingCtx := ast.NewParsingContext(false)
	opts := newEvalOpts()
	m := newModel()

	rule, err := eval.NewRule("check", expr, parsingCtx, opts)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Severity: "error",
			Message:  fmt.Sprintf("parse error: %s", err),
		})
		return result
	}

	if err := rule.GenEvaluator(m); err != nil {
		diag := diagFromEvalError(err)
		result.Diagnostics = append(result.Diagnostics, diag)
		return result
	}

	result.Valid = true
	result.Fields = rule.GetFields()
	sort.Strings(result.Fields)

	eventType, err := rule.GetEventType()
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Severity: "warning",
			Message:  err.Error(),
		})
	} else {
		result.EventType = eventType
	}

	return result
}

func diagFromEvalError(err error) Diagnostic {
	diag := Diagnostic{
		Severity: "error",
		Message:  err.Error(),
	}

	if field := extractFieldFromError(err); field != "" {
		diag.Field = field
		diag.Suggestions = suggestFields(field, "")
	}

	return diag
}

func extractFieldFromError(err error) string {
	msg := err.Error()
	if idx := strings.Index(msg, "field `"); idx >= 0 {
		start := idx + len("field `")
		end := strings.Index(msg[start:], "`")
		if end > 0 {
			return msg[start : start+end]
		}
	}
	return ""
}

// getAllFields returns all SECL fields with their metadata.
func getAllFields() []FieldInfo {
	ev := model.NewFakeEvent()
	fieldNames := ev.GetFields()

	fields := make([]FieldInfo, 0, len(fieldNames))
	for _, name := range fieldNames {
		eventType, kind, basicType, isArray, err := ev.GetFieldMetadata(name)
		if err != nil {
			continue
		}
		typeName := basicType
		if typeName == "" {
			typeName = kind.String()
		}
		fields = append(fields, FieldInfo{
			Name:      name,
			EventType: eventType,
			Type:      typeName,
			IsArray:   isArray,
		})
	}

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})
	return fields
}

func listFields(eventType string) *ListResult {
	fields := getAllFields()
	if eventType != "" {
		filtered := make([]FieldInfo, 0)
		for _, f := range fields {
			if f.EventType == "" || f.EventType == eventType {
				filtered = append(filtered, f)
			}
		}
		fields = filtered
	}
	return &ListResult{Fields: fields}
}

func listEventTypes() *ListResult {
	m := newModel()
	types := m.GetEventTypes()
	strTypes := make([]string, len(types))
	for i, t := range types {
		strTypes[i] = string(t)
	}
	sort.Strings(strTypes)
	return &ListResult{EventTypes: strTypes}
}

func suggestFieldsCmd(partial string, eventType string) *ListResult {
	suggestions := suggestFields(partial, eventType)
	allFields := getAllFields()
	fieldMap := make(map[string]FieldInfo, len(allFields))
	for _, f := range allFields {
		fieldMap[f.Name] = f
	}

	fields := make([]FieldInfo, 0, len(suggestions))
	for _, name := range suggestions {
		if fi, ok := fieldMap[name]; ok {
			fields = append(fields, fi)
		}
	}
	return &ListResult{Fields: fields}
}

// suggestFields returns up to 10 field names that fuzzy-match the given partial.
func suggestFields(partial string, eventType string) []string {
	allFields := getAllFields()
	partial = strings.ToLower(partial)
	parts := strings.Split(partial, ".")

	type scored struct {
		name  string
		score int
	}
	var matches []scored

	for _, fi := range allFields {
		if eventType != "" && fi.EventType != "" && fi.EventType != eventType {
			continue
		}
		name := strings.ToLower(fi.Name)
		score := 0

		// Exact prefix
		if strings.HasPrefix(name, partial) {
			score += 100
		}

		// Contains full partial
		if strings.Contains(name, partial) {
			score += 50
		}

		// Per-segment matching
		fieldParts := strings.Split(name, ".")
		for i, p := range parts {
			if i < len(fieldParts) {
				if strings.HasPrefix(fieldParts[i], p) {
					score += 20
				} else if strings.Contains(fieldParts[i], p) {
					score += 10
				}
			}
		}

		// Same structure, close segments (handles typos)
		if len(parts) == len(fieldParts) {
			allClose := true
			for i := range parts {
				if !closeEnough(parts[i], fieldParts[i]) {
					allClose = false
					break
				}
			}
			if allClose {
				score += 80
			}
		}

		if score > 0 {
			matches = append(matches, scored{fi.Name, score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].name < matches[j].name
	})

	limit := 10
	if len(matches) < limit {
		limit = len(matches)
	}
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = matches[i].name
	}
	return result
}

// closeEnough returns true if two strings have edit distance <= 2.
func closeEnough(a, b string) bool {
	if a == b {
		return true
	}
	la, lb := len(a), len(b)
	if la-lb > 2 || lb-la > 2 {
		return false
	}
	if la > lb {
		a, b = b, a
		la, lb = lb, la
	}
	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for i := range prev {
		prev[i] = i
	}
	for j := 1; j <= lb; j++ {
		curr[0] = j
		minVal := curr[0]
		for i := 1; i <= la; i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[i] = min(curr[i-1]+1, min(prev[i]+1, prev[i-1]+cost))
			if curr[i] < minVal {
				minVal = curr[i]
			}
		}
		if minVal > 2 {
			return false
		}
		prev, curr = curr, prev
	}
	return prev[la] <= 2
}

// serializableConstants returns SECL constants with JSON-serializable values.
func serializableConstants() map[string]interface{} {
	raw := model.SECLConstants()
	out := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		switch e := v.(type) {
		case *eval.IntEvaluator:
			out[k] = e.Value
		case *eval.StringEvaluator:
			out[k] = e.Value
		case *eval.BoolEvaluator:
			out[k] = e.Value
		case *eval.StringArrayEvaluator:
			out[k] = e.Values
		case *eval.IntArrayEvaluator:
			out[k] = e.Values
		case *eval.BoolArrayEvaluator:
			out[k] = e.Values
		case int, int64, string, bool, float64:
			out[k] = v
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
