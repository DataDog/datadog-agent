// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schema

import (
	"errors"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var errNoSchema = errors.New("no embedded schema")

// escapeTokens re-applies RFC 6901 escaping so the pointer we hand out matches what any other
// JSON Schema tool would produce for the same location.
func escapeTokens(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = strings.ReplaceAll(t, "~", "~0")
		t = strings.ReplaceAll(t, "/", "~1")
		out = append(out, t)
	}
	return out
}

// dottedKey renders an instance location the way the customer wrote it in YAML:
// [apm_config enabled] -> apm_config.enabled, [tags 0] -> tags[0].
// A token that is not a bare identifier (a URL used as a map key) is bracketed and quoted so the
// result stays readable and unambiguous.
func dottedKey(tokens []string) string {
	if len(tokens) == 0 {
		return "(root)"
	}
	var b strings.Builder
	for _, t := range tokens {
		switch {
		case isIndex(t):
			b.WriteString("[" + t + "]")
		case isBareIdentifier(t):
			if b.Len() > 0 {
				b.WriteString(".")
			}
			b.WriteString(t)
		default:
			b.WriteString("[" + strconv.Quote(t) + "]")
		}
	}
	return b.String()
}

func isIndex(t string) bool {
	if t == "" {
		return false
	}
	for _, r := range t {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isBareIdentifier(t string) bool {
	if t == "" {
		return false
	}
	for _, r := range t {
		ok := r == '_' ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// schemaNodeAt walks the compiled schema to the node that failed. The compiled form exposes
// Properties, Items and AdditionalProperties, which is enough to reach every leaf this schema
// has — it uses no $ref, oneOf or patternProperties.
func schemaNodeAt(root *jsonschema.Schema, tokens []string) *jsonschema.Schema {
	node := root
	for _, t := range tokens {
		if node == nil {
			return nil
		}
		if isIndex(t) {
			node = itemsOf(node)
			continue
		}
		if child, ok := node.Properties[t]; ok {
			node = child
			continue
		}
		node = additionalOf(node)
	}
	return node
}

// itemsOf resolves the element schema. Our schemas are draft 2020-12, where the library stores
// `items` in Items2020; the untyped Items field holds the pre-2020 form, which is why reading it
// alone silently finds nothing.
func itemsOf(node *jsonschema.Schema) *jsonschema.Schema {
	if node == nil {
		return nil
	}
	if node.Items2020 != nil {
		return node.Items2020
	}
	switch items := node.Items.(type) {
	case *jsonschema.Schema:
		return items
	case []*jsonschema.Schema:
		if len(items) > 0 {
			return items[0]
		}
	}
	return nil
}

// additionalOf normalises AdditionalProperties, which is nil, a bool, or a schema. Every map
// setting in this schema uses the schema form; the bool form carries no type to report.
func additionalOf(node *jsonschema.Schema) *jsonschema.Schema {
	if node == nil {
		return nil
	}
	if sch, ok := node.AdditionalProperties.(*jsonschema.Schema); ok {
		return sch
	}
	return nil
}

func typeOf(node *jsonschema.Schema) string {
	if node == nil || node.Types == nil {
		return ""
	}
	if ts := node.Types.ToStrings(); len(ts) == 1 {
		return ts[0]
	}
	return ""
}

// valueAt walks the validated instance to the offending value. ValidationError carries the
// location but not the value, and the caller already holds the instance, so this costs one walk.
func valueAt(config interface{}, tokens []string) interface{} {
	current := config
	for _, t := range tokens {
		switch node := current.(type) {
		case map[string]interface{}:
			v, ok := node[t]
			if !ok {
				return nil
			}
			current = v
		case []interface{}:
			i, err := strconv.Atoi(t)
			if err != nil || i < 0 || i >= len(node) {
				return nil
			}
			current = node[i]
		default:
			return nil
		}
	}
	return current
}
