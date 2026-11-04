// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
	"github.com/valyala/fastjson"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
)

// OTTLCondition is an upstream OTTL predicate compiled for the Agent log context.
// Conditions are immutable after compilation and may be shared across pipelines.
type OTTLCondition = ottl.Condition[*OTTLContext]

// OTTLContext provides read-only access to a message's body and JSON attributes.
// It parses JSON lazily, once per content version, and materializes only the values
// requested by a condition. It must not be used concurrently or retained by a rule.
type OTTLContext struct {
	parser   fastjson.Parser
	content  []byte
	root     *fastjson.Value
	parseErr error
	parsed   bool
	body     string
	bodySet  bool
}

var ottlContexts = sync.Pool{New: func() any { return &OTTLContext{} }}

// AcquireOTTLContext borrows a context for one message. The caller must release it.
func AcquireOTTLContext(content []byte) *OTTLContext {
	c := ottlContexts.Get().(*OTTLContext)
	c.Reset(content)
	return c
}

// ReleaseOTTLContext returns a context without retaining the original message.
func ReleaseOTTLContext(c *OTTLContext) {
	c.Reset(nil)
	ottlContexts.Put(c)
}

// Reset invalidates parsed attributes after a preceding rule changes the content.
// The parser's allocation arena is retained for reuse across messages.
func (c *OTTLContext) Reset(content []byte) {
	c.content = content
	c.root = nil
	c.parseErr = nil
	c.parsed = false
	c.body = ""
	c.bodySet = false
}

func (c *OTTLContext) attributes() (*fastjson.Value, error) {
	if !c.parsed {
		c.parsed = true
		c.root, c.parseErr = c.parser.ParseBytes(c.content)
		if c.parseErr == nil {
			// ParseBytes deliberately accepts malformed escapes and some non-JSON
			// numbers. Validate before a predicate can decide to drop a message.
			c.parseErr = fastjson.ValidateBytes(c.content)
		}
		if c.parseErr == nil && c.root.Type() != fastjson.TypeObject {
			c.parseErr = errors.New("OTTL attributes require a JSON object")
		}
	}
	return c.root, c.parseErr
}

// Register converters individually: StandardFuncs retains all editors and parsers
// (including XML, grok and hashing), even though filtering never uses them.
func ottlFilterFunctions() map[string]ottl.Factory[*OTTLContext] {
	return ottl.CreateFactoryMap(
		ottlfuncs.NewIsMatchFactory[*OTTLContext](),
		ottlfuncs.NewHasPrefixFactory[*OTTLContext](),
		ottlfuncs.NewHasSuffixFactory[*OTTLContext](),
		ottlfuncs.NewIsStringFactory[*OTTLContext](),
		ottlfuncs.NewIsIntFactory[*OTTLContext](),
		ottlfuncs.NewIsDoubleFactory[*OTTLContext](),
		ottlfuncs.NewIsBoolFactory[*OTTLContext](),
		ottlfuncs.NewIsMapFactory[*OTTLContext](),
		ottlfuncs.NewIsListFactory[*OTTLContext](),
		ottlfuncs.NewLenFactory[*OTTLContext](),
		ottlfuncs.NewIntFactory[*OTTLContext](),
		ottlfuncs.NewDoubleFactory[*OTTLContext](),
		ottlfuncs.NewStringFactory[*OTTLContext](),
	)
}

func compileOTTLCondition(pattern string) (*OTTLCondition, error) {
	parser, err := ottl.NewParser(ottlFilterFunctions(), parseOTTLPath, component.TelemetrySettings{Logger: zap.NewNop()})
	if err != nil {
		return nil, err
	}
	return parser.ParseCondition(pattern)
}

type ottlJSONKey struct {
	name    string
	index   int64
	isIndex bool
	dynamic ottl.Getter[*OTTLContext]
}

func parseOTTLPath(path ottl.Path[*OTTLContext]) (ottl.GetSetter[*OTTLContext], error) {
	if path.Name() == "log" && len(path.Keys()) == 0 && path.Next() != nil {
		path = path.Next()
	}
	if path.Next() != nil {
		return nil, fmt.Errorf("unsupported OTTL path %q: use bracket notation for JSON fields", path.String())
	}
	readOnly := func(context.Context, *OTTLContext, any) error {
		return errors.New("Agent OTTL filter paths are read-only")
	}
	if path.Name() == "body" && len(path.Keys()) == 0 {
		return ottl.StandardGetSetter[*OTTLContext]{Getter: func(_ context.Context, c *OTTLContext) (any, error) {
			if !c.bodySet {
				c.body, c.bodySet = string(c.content), true
			}
			return c.body, nil
		}, Setter: readOnly}, nil
	}
	if path.Name() != "attributes" {
		return nil, fmt.Errorf("unsupported OTTL path %q: expected body or attributes", path.String())
	}
	keys := make([]ottlJSONKey, 0, len(path.Keys()))
	for _, key := range path.Keys() {
		// Literal keys are resolved at compilation; dynamic keys retain their getter.
		getter, err := key.ExpressionGetter(context.Background(), nil)
		if err != nil {
			return nil, err
		}
		if getter != nil {
			keys = append(keys, ottlJSONKey{dynamic: getter})
			continue
		}
		name, err := key.String(context.Background(), nil)
		if err != nil {
			return nil, err
		}
		if name != nil {
			keys = append(keys, ottlJSONKey{name: *name})
			continue
		}
		index, err := key.Int(context.Background(), nil)
		if err != nil {
			return nil, err
		}
		if index == nil {
			return nil, fmt.Errorf("unsupported key in OTTL path %q", path.String())
		}
		keys = append(keys, ottlJSONKey{index: *index, isIndex: true})
	}
	return ottl.StandardGetSetter[*OTTLContext]{Getter: func(ctx context.Context, c *OTTLContext) (any, error) {
		value, err := c.attributes()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if value == nil || value.Type() == fastjson.TypeNull {
				return nil, nil
			}
			if key.dynamic != nil {
				resolved, err := key.dynamic.Get(ctx, c)
				if err != nil {
					return nil, err
				}
				switch k := resolved.(type) {
				case string:
					key.name, key.isIndex = k, false
				case int64:
					key.index, key.isIndex = k, true
				default:
					return nil, fmt.Errorf("OTTL index must be a string or integer, got %T", resolved)
				}
			}
			if key.isIndex {
				array, err := value.Array()
				if err != nil {
					return nil, err
				}
				if key.index < 0 || key.index >= int64(len(array)) {
					return nil, errors.New("OTTL array index out of bounds")
				}
				value = array[key.index]
			} else {
				object, err := value.Object()
				if err != nil {
					return nil, err
				}
				// Match encoding/json's last-key-wins behavior, including escaped
				// duplicate keys. Object.Get would return the first occurrence.
				value = nil
				object.Visit(func(name []byte, candidate *fastjson.Value) {
					if string(name) == key.name {
						value = candidate
					}
				})
			}
		}
		return ottlJSONValue(value)
	}, Setter: readOnly}, nil
}

func ottlJSONValue(value *fastjson.Value) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch value.Type() {
	case fastjson.TypeNull:
		return nil, nil
	case fastjson.TypeString:
		b, err := value.StringBytes()
		return string(b), err
	case fastjson.TypeTrue:
		return true, nil
	case fastjson.TypeFalse:
		return false, nil
	case fastjson.TypeNumber:
		if n, err := value.Int64(); err == nil {
			return n, nil
		}
		// Never round an integer ID through float64 just because it exceeds int64.
		if !strings.ContainsAny(value.String(), ".eE") {
			return nil, errors.New("JSON integer exceeds the OTTL int64 range")
		}
		return value.Float64()
	case fastjson.TypeArray:
		array, _ := value.Array()
		result := make([]any, len(array))
		for i, v := range array {
			var err error
			result[i], err = ottlJSONValue(v)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	case fastjson.TypeObject:
		object, _ := value.Object()
		result := make(map[string]any, object.Len())
		var err error
		object.Visit(func(key []byte, v *fastjson.Value) {
			if err == nil {
				result[string(key)], err = ottlJSONValue(v)
			}
		})
		return result, err
	default:
		return nil, fmt.Errorf("unsupported JSON value type %s", value.Type())
	}
}
