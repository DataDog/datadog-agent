// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/spf13/cast"
)

func splitKey(key string) []string {
	return strings.Split(strings.ToLower(key), ".")
}

func joinKey(parts ...string) string {
	nonEmptyParts := make([]string, 0, len(parts))
	for idx := range parts {
		if parts[idx] == "" {
			continue
		}
		nonEmptyParts = append(nonEmptyParts, parts[idx])
	}
	return strings.Join(nonEmptyParts, ".")
}

func safeMul(a, b uint) uint {
	c := a * b
	// detect multiplication overflows
	if a > 1 && b > 1 && c/b != a {
		return 0
	}
	return c
}

// parseSizeInBytes converts strings like 1GB or 12 mb into an unsigned integer number of bytes.
func parseSizeInBytes(sizeStr string) uint {
	sizeStr = strings.TrimSpace(sizeStr)
	lastChar := len(sizeStr) - 1
	multiplier := uint(1)

	if lastChar > 0 {
		if sizeStr[lastChar] == 'b' || sizeStr[lastChar] == 'B' {
			if lastChar > 1 {
				switch unicode.ToLower(rune(sizeStr[lastChar-1])) {
				case 'k':
					multiplier = 1 << 10
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				case 'm':
					multiplier = 1 << 20
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				case 'g':
					multiplier = 1 << 30
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				default:
					multiplier = 1
					sizeStr = strings.TrimSpace(sizeStr[:lastChar])
				}
			}
		}
	}

	size := max(cast.ToInt(sizeStr), 0)

	return safeMul(uint(size), multiplier)
}

// ToMapStringInterface converts any type of map into a map[string]interface{}
func ToMapStringInterface(data any, path string) (map[string]interface{}, error) {
	if res, ok := data.(map[string]interface{}); ok {
		return res, nil
	}

	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Map:
		convert := map[string]interface{}{}
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			switch k := key.Interface().(type) {
			case string:
				convert[k] = iter.Value().Interface()
			default:
				convert[fmt.Sprintf("%v", key.Interface())] = iter.Value().Interface()
			}
		}
		return convert, nil
	}
	return nil, fmt.Errorf("expected map at '%s' got: %v", path, v)
}
