// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"fmt"
	"reflect"
	"strconv"
)

// Decode populates target (pointer to the struct the schema came from) from a
// map of string values, applying defaults and validating required/enum/kind.
func Decode(s Schema, values map[string]string, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("Decode: want pointer to struct, got %T", target)
	}
	known := map[string]struct{}{}
	for _, f := range s.Fields {
		known[f.Name] = struct{}{}
	}
	for k := range values {
		if _, ok := known[k]; !ok {
			return fmt.Errorf("unknown option %q", k)
		}
	}
	// Defaults first, in one place.
	if err := ApplyDefaults(s, target); err != nil {
		return err
	}
	// Overlay only provided keys.
	elem := rv.Elem()
	for _, f := range s.Fields {
		raw, present := values[f.Name]
		if !present {
			if f.Required {
				return fmt.Errorf("missing required option %q", f.Name)
			}
			continue
		}
		if len(f.Enum) > 0 && !contains(f.Enum, raw) {
			return fmt.Errorf("option %q: %q not in [%v]", f.Name, raw, f.Enum)
		}
		if err := setValue(elem.FieldByIndex(f.Index), f.Kind, raw); err != nil {
			return fmt.Errorf("option %q: %w", f.Name, err)
		}
	}
	return nil
}

func setValue(fv reflect.Value, kind Kind, raw string) error {
	switch kind {
	case KindString:
		fv.SetString(raw)
	case KindBool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid bool %q", raw)
		}
		fv.SetBool(b)
	case KindInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid int %q", raw)
		}
		fv.SetInt(n)
	default:
		return fmt.Errorf("unsupported kind %s", kind)
	}
	return nil
}

// ApplyDefaults sets every field that declares a `default` tag to that default.
// Fields without a default are left at their zero value.
func ApplyDefaults(s Schema, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("ApplyDefaults: want pointer to struct, got %T", target)
	}
	elem := rv.Elem()
	for _, f := range s.Fields {
		if f.Default == "" {
			continue
		}
		if err := setValue(elem.FieldByIndex(f.Index), f.Kind, f.Default); err != nil {
			return fmt.Errorf("option %q: %w", f.Name, err)
		}
	}
	return nil
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
