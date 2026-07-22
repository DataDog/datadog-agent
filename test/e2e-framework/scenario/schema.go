// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package scenario provides a reflection-based model for defining e2e scenarios
// once and driving them from tests, a CLI, and a service.
package scenario

import (
	"fmt"
	"reflect"
	"strings"
)

// Kind is the supported flag/field kind.
type Kind string

const (
	KindString Kind = "string"
	KindBool   Kind = "bool"
	KindInt    Kind = "int"
)

// Field is one introspectable scenario parameter.
type Field struct {
	Name     string   `json:"name"`              // CLI flag / config key
	GoName   string   `json:"goName"`            // Go struct field name
	Kind     Kind     `json:"kind"`
	Default  string   `json:"default,omitempty"`
	Help     string   `json:"help,omitempty"`
	Enum     []string `json:"enum,omitempty"`
	Required bool     `json:"required,omitempty"`
	Index    []int    `json:"index"` // reflect field index path (supports nested components)
}

// Schema is the ordered set of a struct's introspectable fields.
type Schema struct {
	Fields []Field `json:"fields"`
}

// BuildSchema reflects a pointer-to-struct into a Schema, recursing into nested
// struct fields (reusable param components) and skipping `scenario:"-"` and
// untagged fields.
//
// It returns an error if two fields share the same flag name, which would
// otherwise cause silent collision bugs at decode time.
func BuildSchema(v any) (Schema, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return Schema{}, fmt.Errorf("BuildSchema: want pointer to struct, got %T", v)
	}
	var s Schema
	if err := walk(rv.Elem().Type(), nil, &s); err != nil {
		return Schema{}, err
	}
	seen := make(map[string]bool, len(s.Fields))
	for _, f := range s.Fields {
		if seen[f.Name] {
			return Schema{}, fmt.Errorf("duplicate scenario flag name %q", f.Name)
		}
		seen[f.Name] = true
	}
	return s, nil
}

func walk(t reflect.Type, prefix []int, s *Schema) error {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		idx := append(append([]int{}, prefix...), i)
		tag, hasTag := sf.Tag.Lookup("scenario")
		if hasTag && strings.TrimSpace(tag) == "-" {
			continue // escape hatch
		}
		// Recurse into struct-typed fields (reusable components).
		if sf.Type.Kind() == reflect.Struct {
			if err := walk(sf.Type, idx, s); err != nil {
				return err
			}
			continue
		}
		if !hasTag {
			continue // not introspectable
		}
		info := parseTag(tag)
		kind, err := kindOf(sf.Type)
		if err != nil {
			return fmt.Errorf("field %s: %w", sf.Name, err)
		}
		s.Fields = append(s.Fields, Field{
			Name:     info.name,
			GoName:   sf.Name,
			Kind:     kind,
			Default:  info.def,
			Help:     info.help,
			Enum:     info.enum,
			Required: info.required,
			Index:    idx,
		})
	}
	return nil
}

func kindOf(t reflect.Type) (Kind, error) {
	switch t.Kind() {
	case reflect.String:
		return KindString, nil
	case reflect.Bool:
		return KindBool, nil
	case reflect.Int, reflect.Int64:
		return KindInt, nil
	default:
		return "", fmt.Errorf("unsupported tagged field kind %s", t.Kind())
	}
}

type tagInfo struct {
	name     string
	def      string
	help     string
	enum     []string
	required bool
}

func parseTag(tag string) tagInfo {
	var info tagInfo
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, hasEq := strings.Cut(part, "=")
		switch strings.TrimSpace(key) {
		case "name":
			info.name = val
		case "default":
			info.def = val
		case "help":
			info.help = val
		case "enum":
			if hasEq && val != "" {
				info.enum = strings.Split(val, "|")
			}
		case "required":
			info.required = true
		}
	}
	return info
}
