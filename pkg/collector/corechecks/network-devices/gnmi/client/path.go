// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

// CacheKey identifies a cached value by normalized path and gNMI path keys.
type CacheKey struct {
	Path string
	Keys map[string]string
}

func (k CacheKey) cacheKey() string {
	return strconv.Quote(k.Path) + "{" + formatQuotedKeys(k.Keys) + "}"
}

// String returns a stable string representation for logging and debugging.
func (k CacheKey) String() string {
	return fmt.Sprintf("%s{%s}", k.Path, formatKeys(k.Keys))
}

func formatKeys(keys map[string]string) string {
	return formatKeysWith(keys, func(value string) string { return value })
}

func formatQuotedKeys(keys map[string]string) string {
	return formatKeysWith(keys, strconv.Quote)
}

func formatKeysWith(keys map[string]string, format func(string) string) string {
	if len(keys) == 0 {
		return ""
	}

	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, format(name)+"="+format(keys[name]))
	}
	return strings.Join(parts, ",")
}

type pathElement struct {
	name string
	keys map[string]string
}

type normalizedPath struct {
	origin   string
	target   string
	elements []pathElement
}

func normalizedPathFromGNMIPath(path *gnmipb.Path) normalizedPath {
	if path == nil {
		return normalizedPath{}
	}

	elements := make([]pathElement, 0, len(path.GetElem()))
	for _, elem := range path.GetElem() {
		elements = append(elements, pathElement{
			name: elem.GetName(),
			keys: cloneKeys(elem.GetKey()),
		})
	}
	return normalizedPath{
		origin:   path.GetOrigin(),
		target:   path.GetTarget(),
		elements: elements,
	}
}

func (p normalizedPath) id() string {
	var builder strings.Builder
	builder.WriteString("origin=")
	builder.WriteString(strconv.Quote(p.origin))
	builder.WriteString(",target=")
	builder.WriteString(strconv.Quote(p.target))
	for _, elem := range p.elements {
		builder.WriteByte('/')
		builder.WriteString(strconv.Quote(elem.name))
		builder.WriteByte('{')
		builder.WriteString(formatQuotedKeys(elem.keys))
		builder.WriteByte('}')
	}
	return builder.String()
}

func (p normalizedPath) cacheKey() CacheKey {
	segments := make([]string, 0, len(p.elements))
	keys := make(map[string]string)
	for _, elem := range p.elements {
		segments = append(segments, elem.name)
		for name, value := range elem.keys {
			keys[name] = value
		}
	}
	if len(keys) == 0 {
		keys = nil
	}
	return CacheKey{Path: "/" + strings.Join(segments, "/"), Keys: keys}
}

func (p normalizedPath) matchesPrefix(prefix normalizedPath) bool {
	if prefix.origin != "" && p.origin != prefix.origin {
		return false
	}
	if prefix.target != "" && p.target != prefix.target {
		return false
	}
	if len(prefix.elements) > len(p.elements) {
		return false
	}
	for i, prefixElem := range prefix.elements {
		elem := p.elements[i]
		if elem.name != prefixElem.name {
			return false
		}
		for name, value := range prefixElem.keys {
			if elem.keys[name] != value {
				return false
			}
		}
	}
	return true
}

func cacheKeyFromGNMIPath(path *gnmipb.Path) CacheKey {
	return normalizedPathFromGNMIPath(path).cacheKey()
}

func joinGNMIPaths(prefix, path *gnmipb.Path) *gnmipb.Path {
	joined := &gnmipb.Path{}
	if prefix != nil {
		joined.Origin = prefix.GetOrigin()
		joined.Target = prefix.GetTarget()
		joined.Elem = append(joined.Elem, prefix.GetElem()...)
	}
	if path != nil {
		if path.GetOrigin() != "" {
			joined.Origin = path.GetOrigin()
		}
		if path.GetTarget() != "" {
			joined.Target = path.GetTarget()
		}
		joined.Elem = append(joined.Elem, path.GetElem()...)
	}
	return joined
}

func subscribePathFromMetric(metric config.MetricConfig) (*gnmipb.Path, error) {
	segments, err := splitProfilePath(metric.Path)
	if err != nil {
		return nil, err
	}

	elems := make([]*gnmipb.PathElem, 0, len(segments))
	for _, segment := range segments {
		elem := &gnmipb.PathElem{Name: segment}
		if keyName, ok := metric.Tags[segment]; ok && keyName != "" {
			elem.Key = map[string]string{keyName: "*"}
		}
		elems = append(elems, elem)
	}

	return &gnmipb.Path{Elem: elems}, nil
}

func splitProfilePath(path string) ([]string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, errors.New("path must not be empty")
	}

	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return nil, errors.New("path must not be empty")
	}

	segments := strings.Split(trimmed, "/")
	for _, segment := range segments {
		if segment == "" {
			return nil, fmt.Errorf("invalid path %q: empty segment", path)
		}
	}
	return segments, nil
}
