// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"errors"
	"fmt"
	"sort"
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
	return k.Path + "{" + formatKeys(k.Keys) + "}"
}

// String returns a stable string representation for logging and debugging.
func (k CacheKey) String() string {
	return fmt.Sprintf("%s{%s}", k.Path, formatKeys(k.Keys))
}

func formatKeys(keys map[string]string) string {
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
		parts = append(parts, fmt.Sprintf("%s=%s", name, keys[name]))
	}
	return strings.Join(parts, ",")
}

func cacheKeyFromGNMIPath(path *gnmipb.Path) CacheKey {
	normalized, keys := normalizeGNMIPath(path)
	return CacheKey{
		Path: normalized,
		Keys: keys,
	}
}

func normalizeGNMIPath(path *gnmipb.Path) (string, map[string]string) {
	if path == nil {
		return "", nil
	}

	segments := make([]string, 0, len(path.GetElem()))
	keys := make(map[string]string)
	for _, elem := range path.GetElem() {
		segments = append(segments, elem.GetName())
		for key, value := range elem.GetKey() {
			keys[key] = value
		}
	}

	return "/" + strings.Join(segments, "/"), keys
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

func cacheKeyMatchesPrefix(key CacheKey, prefix CacheKey) bool {
	if !strings.HasPrefix(key.Path, prefix.Path) {
		return false
	}
	if len(prefix.Keys) == 0 {
		return true
	}
	for name, value := range prefix.Keys {
		if key.Keys[name] != value {
			return false
		}
	}
	return true
}
