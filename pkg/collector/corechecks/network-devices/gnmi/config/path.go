// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"fmt"
	"strconv"
	"strings"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

// NormalizePath preserves an explicit origin:/path while accepting older paths
// without a leading slash. It does not rewrite response paths or namespaces.
func NormalizePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") || originSeparator(value) >= 0 {
		return value
	}
	return "/" + value
}

// originSeparator only recognizes a top-level origin, never a URL in a key.
func originSeparator(value string) int {
	index := strings.Index(value, ":/")
	if index < 0 || strings.ContainsAny(value[:index], "/[]") {
		return -1
	}
	return index
}

// CompilePath compiles a profile selector into protocol-native ordered elements.
// The historical /openconfig wrapper is accepted only at this configuration
// boundary. Explicit origins always win: native:/openconfig/x retains that root.
// List predicates can express composite keys and values containing slashes.
// Legacy segment -> key-name bindings add wildcard keys without flattening them.
func CompilePath(value string, bindings map[string]string) (*gnmipb.Path, error) {
	value = NormalizePath(value)
	path := &gnmipb.Path{}
	explicitOrigin := false
	if index := originSeparator(value); index >= 0 {
		path.Origin, value = value[:index], value[index+1:]
		if path.Origin == "" || strings.ContainsAny(path.Origin, "/[] \t\r\n:") {
			return nil, fmt.Errorf("invalid path origin %q", path.Origin)
		}
		explicitOrigin = true
	}
	segments, err := splitPathElements(strings.TrimPrefix(value, "/"))
	if err != nil {
		return nil, err
	}
	if !explicitOrigin && segments[0] == "openconfig" {
		path.Origin = "openconfig"
		segments = segments[1:]
		if len(segments) == 0 {
			return nil, fmt.Errorf("OpenConfig selector must name a schema node")
		}
	}
	bound := make(map[string]bool)
	for _, segment := range segments {
		elem, err := parsePathElement(segment)
		if err != nil {
			return nil, err
		}
		if keyName := bindings[elem.Name]; keyName != "" {
			if bound[elem.Name] {
				return nil, fmt.Errorf("ambiguous key binding for repeated element %q; use explicit predicates", elem.Name)
			}
			bound[elem.Name] = true
			if elem.Key == nil {
				elem.Key = make(map[string]string)
			}
			if _, exists := elem.Key[keyName]; !exists {
				elem.Key[keyName] = "*"
			}
		}
		path.Elem = append(path.Elem, elem)
	}
	return path, nil
}

func splitPathElements(value string) ([]string, error) {
	var elements []string
	start, depth := 0, 0
	quoted, escaped := false, false
	for index, char := range value {
		if escaped {
			escaped = false
			continue
		}
		if quoted && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch char {
		case '[':
			depth++
			if depth != 1 {
				return nil, fmt.Errorf("nested predicate in path %q", value)
			}
		case ']':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced predicate in path %q", value)
			}
		case '/':
			if depth == 0 {
				elements = append(elements, value[start:index])
				start = index + 1
			}
		}
	}
	if depth != 0 || quoted || escaped {
		return nil, fmt.Errorf("unterminated predicate in path %q", value)
	}
	elements = append(elements, value[start:])
	for _, element := range elements {
		if element == "" {
			return nil, fmt.Errorf("path must not contain empty elements")
		}
	}
	return elements, nil
}

func parsePathElement(value string) (*gnmipb.PathElem, error) {
	name, tail, _ := strings.Cut(value, "[")
	if name == "" || strings.ContainsAny(name, "]\" \t\r\n") {
		return nil, fmt.Errorf("invalid path element %q", value)
	}
	elem := &gnmipb.PathElem{Name: name}
	if tail == "" && value == name {
		return elem, nil
	}
	elem.Key = make(map[string]string)
	for tail != "" {
		// A quoted JSON string may contain a closing bracket.
		quoted, escaped, end := false, false, -1
		for index, char := range tail {
			if escaped {
				escaped = false
				continue
			}
			if quoted && char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				quoted = !quoted
			}
			if char == ']' && !quoted {
				end = index
				break
			}
		}
		if end < 0 {
			return nil, fmt.Errorf("unterminated predicate in %q", value)
		}
		key, keyValue, found := strings.Cut(tail[:end], "=")
		if !found || key == "" || keyValue == "" || strings.ContainsAny(key, "[]\" \t\r\n") {
			return nil, fmt.Errorf("invalid predicate in %q", value)
		}
		if strings.HasPrefix(keyValue, "\"") {
			decoded, err := strconv.Unquote(keyValue)
			if err != nil {
				return nil, fmt.Errorf("invalid quoted key in %q: %w", value, err)
			}
			keyValue = decoded
		} else if strings.ContainsAny(keyValue, "\"\r\n") {
			return nil, fmt.Errorf("invalid key value in %q", value)
		}
		if _, exists := elem.Key[key]; exists {
			return nil, fmt.Errorf("duplicate key %q in element %q", key, name)
		}
		elem.Key[key] = keyValue
		tail = tail[end+1:]
		if tail != "" {
			if !strings.HasPrefix(tail, "[") {
				return nil, fmt.Errorf("unexpected text after predicate in %q", value)
			}
			tail = tail[1:]
		}
	}
	return elem, nil
}
