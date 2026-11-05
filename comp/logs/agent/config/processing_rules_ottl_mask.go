// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
	"github.com/valyala/fastjson"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
)

func compileOTTLMaskStatement(pattern string) (*OTTLStatement, error) {
	functions := ottlFilterFunctions()
	for _, factory := range []ottl.Factory[*OTTLContext]{
		ottlfuncs.NewReplacePatternFactory[*OTTLContext](),
		ottlfuncs.NewReplaceMatchFactory[*OTTLContext](),
		ottlfuncs.NewSetFactory[*OTTLContext](),
	} {
		functions[factory.Name()] = factory
	}
	parser, err := ottl.NewParser(functions, parseOTTLMaskPath, component.TelemetrySettings{Logger: zap.NewNop()})
	if err != nil {
		return nil, err
	}
	return parser.ParseStatement(pattern)
}

// ApplyMask executes one statement and reports actual content changes. The
// caller must drop the message on error, as with the original mask_ottl rule.
func (c *OTTLContext) ApplyMask(ctx context.Context, statement *OTTLStatement) (bool, error) {
	before := c.revision
	_, executed, err := statement.Execute(ctx, c)
	// Even a no-op field mask must not retain shadowed duplicate values in
	// the original bytes. A false where clause still leaves content untouched.
	if err == nil && executed && c.parsed && c.parseErr == nil && !c.jsonDirty && !c.canonical {
		if !c.duplicatesChecked {
			c.duplicates = ottlJSONHasDuplicates(c.root)
			c.duplicatesChecked = true
		}
		if c.duplicates {
			c.jsonDirty = true
			c.revision++
		}
	}
	return c.revision != before, err
}

// Content returns message-owned bytes. Adjacent JSON masks and conditions use
// the cached tree; bytes are materialized only for a body/legacy rule or output.
// Returned bytes remain valid after the context is released to the pool.
func (c *OTTLContext) Content() []byte {
	if c.jsonDirty {
		c.content = c.marshalJSON(make([]byte, 0, len(c.content)), c.root)
		c.jsonDirty = false
		c.canonical = true
		c.body = ""
		c.bodySet = false
	}
	return c.content
}

func (c *OTTLContext) setBody(value any) error {
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("OTTL body requires a string, got %T", value)
	}
	if string(c.Content()) == text {
		return nil
	}
	revision := c.revision
	c.Reset([]byte(text))
	c.revision = revision + 1
	return nil
}

func (c *OTTLContext) setJSONValue(ctx context.Context, keys []ottlJSONKey, value any) error {
	if len(keys) == 0 {
		return errors.New("OTTL masking requires a JSON field or array element")
	}
	parent, err := c.jsonPathValue(ctx, keys[:len(keys)-1])
	if err != nil {
		return err
	}
	if parent == nil || parent.Type() == fastjson.TypeNull {
		return errors.New("OTTL masking target has a missing parent")
	}
	key, err := keys[len(keys)-1].resolve(ctx, c)
	if err != nil {
		return err
	}
	original, err := ottlJSONChild(parent, key)
	if err != nil {
		return err
	}
	current := c.replacedChild(parent, key, original)
	if current != nil && current.Type() != fastjson.TypeObject && current.Type() != fastjson.TypeArray {
		old, err := c.ottlJSONValue(current)
		// Current is scalar here, so equality never compares maps or slices.
		if err == nil && old == value {
			return nil
		}
	}
	replacement, err := c.jsonScalar(value)
	if err != nil {
		return err
	}
	if original == nil {
		// A missing final object field may be added; parents and array indexes
		// must already exist. The path resolver has checked the parent type.
		parent.Set(key.name, replacement)
	} else {
		if c.replacements == nil {
			c.replacements = make(map[ottlJSONLocation]*fastjson.Value)
		}
		c.replacements[key.location(parent)] = replacement
	}
	c.jsonDirty = true
	c.body = ""
	c.bodySet = false
	c.revision++
	return nil
}

func (c *OTTLContext) jsonScalar(value any) (*fastjson.Value, error) {
	switch v := value.(type) {
	case nil:
		return c.arena.NewNull(), nil
	case string:
		return c.arena.NewString(v), nil
	case bool:
		if v {
			return c.arena.NewTrue(), nil
		}
		return c.arena.NewFalse(), nil
	case int64:
		return c.arena.NewNumberString(strconv.FormatInt(v, 10)), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("OTTL masking cannot write a non-finite JSON number")
		}
		number := strconv.FormatFloat(v, 'g', -1, 64)
		if !strings.ContainsAny(number, ".eE") {
			number += ".0"
		}
		return c.arena.NewNumberString(number), nil
	default:
		return nil, fmt.Errorf("OTTL masking can assign only JSON scalars, got %T", value)
	}
}

// Emit the last occurrence of duplicate keys, matching attribute reads. This
// prevents shadowed, unmasked copies from surviving a transformed document.
// Untouched numbers are serialized directly, without a float64 round trip.
func (c *OTTLContext) marshalJSON(dst []byte, value *fastjson.Value) []byte {
	switch value.Type() {
	case fastjson.TypeArray:
		array, _ := value.Array()
		dst = append(dst, '[')
		for i, child := range array {
			if i != 0 {
				dst = append(dst, ',')
			}
			dst = c.marshalJSON(dst, c.replacedChild(value, ottlJSONKey{index: int64(i), isIndex: true}, child))
		}
		return append(dst, ']')
	case fastjson.TypeObject:
		object, _ := value.Object()
		last := make(map[string]*fastjson.Value, object.Len())
		object.Visit(func(key []byte, child *fastjson.Value) {
			last[string(key)] = child
		})
		dst = append(dst, '{')
		first := true
		object.Visit(func(key []byte, child *fastjson.Value) {
			if last[string(key)] != child {
				return
			}
			delete(last, string(key))
			if !first {
				dst = append(dst, ',')
			}
			first = false
			dst = c.arena.NewStringBytes(key).MarshalTo(dst)
			dst = append(dst, ':')
			dst = c.marshalJSON(dst, c.replacedChild(value, ottlJSONKey{name: string(key)}, child))
		})
		return append(dst, '}')
	default:
		return value.MarshalTo(dst)
	}
}

func ottlJSONHasDuplicates(value *fastjson.Value) bool {
	switch value.Type() {
	case fastjson.TypeArray:
		array, _ := value.Array()
		for _, child := range array {
			if ottlJSONHasDuplicates(child) {
				return true
			}
		}
	case fastjson.TypeObject:
		object, _ := value.Object()
		seen := make(map[string]struct{}, object.Len())
		duplicate := false
		object.Visit(func(key []byte, child *fastjson.Value) {
			if duplicate {
				return
			}
			if _, exists := seen[string(key)]; exists {
				duplicate = true
				return
			}
			seen[string(key)] = struct{}{}
			duplicate = ottlJSONHasDuplicates(child)
		})
		return duplicate
	}
	return false
}
