// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package nodetreemodel

import (
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// GetConfigTypeConversions returns only conversions belonging to the effective settings.
// Keeping them on the source leaf also handles equal-value edits and removed overrides.
func (c *ntmConfig) GetConfigTypeConversions() []model.ConfigTypeConversion {
	c.maybeRebuild()
	c.RLock()
	defer c.RUnlock()
	var result []model.ConfigTypeConversion
	var visit func(*nodeImpl)
	visit = func(node *nodeImpl) {
		result = append(result, node.conversions...)
		for _, key := range node.ChildrenKeys() {
			visit(node.children[key])
		}
	}
	visit(c.root)
	return result
}

func typeConversions(key, path string, source model.Source, before, after any) []model.ConfigTypeConversion {
	switch source {
	case model.SourceFile, model.SourceFleetPolicies, model.SourceSecret, model.SourceRC, model.SourceCLI:
	default:
		return nil
	}
	if text, ok := before.(string); ok && source != model.SourceSecret && scrubber.IsEnc(text) {
		return nil
	}
	from, to := configType(before), configType(after)
	if from == "" || to == "" {
		return nil
	}
	if from != to {
		return []model.ConfigTypeConversion{{Key: key, Path: path, Source: source, FromType: from, ToType: to}}
	}
	if from != "array" {
		return nil
	}
	a, b := reflect.ValueOf(before), reflect.ValueOf(after)
	var result []model.ConfigTypeConversion
	for i := 0; i < min(a.Len(), b.Len()); i++ {
		result = append(result, typeConversions(key, path+"/"+strconv.Itoa(i), source, a.Index(i).Interface(), b.Index(i).Interface())...)
	}
	return result
}

// Treat Go numeric widths alike; duration and timestamp parsing are supported syntax.
func configType(value any) string {
	switch value.(type) {
	case time.Duration, time.Time:
		return ""
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		if !math.IsInf(v.Float(), 0) && math.Trunc(v.Float()) == v.Float() {
			return "integer"
		}
		return "number"
	case reflect.Array, reflect.Slice:
		return "array"
	case reflect.Map:
		return "object"
	default:
		return ""
	}
}
