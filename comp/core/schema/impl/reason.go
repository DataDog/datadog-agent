// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schemaimpl

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// violationReason describes a validation error from the schema side only. Every kind carries the offending value
// (Got, AdditionalProperties.Properties, PropertyNames.Property, Format.Err...): none of it may be used here, as the
// reason is reported as telemetry.
func violationReason(errorKind jsonschema.ErrorKind) string {
	switch k := errorKind.(type) {
	case *kind.Type:
		return fmt.Sprintf("got %s instead of %s", k.Got, joinOr(k.Want))
	case *kind.Enum:
		values := make([]string, 0, len(k.Want))
		for _, value := range k.Want {
			values = append(values, schemaValue(value))
		}
		return "does not match " + joinOr(values)
	case *kind.Const:
		return "must be " + schemaValue(k.Want)
	case *kind.Pattern:
		return "does not match pattern " + k.Want
	case *kind.Format:
		return "is not a valid " + k.Want
	case *kind.Minimum:
		return "must be >= " + ratString(k.Want)
	case *kind.Maximum:
		return "must be <= " + ratString(k.Want)
	case *kind.ExclusiveMinimum:
		return "must be > " + ratString(k.Want)
	case *kind.ExclusiveMaximum:
		return "must be < " + ratString(k.Want)
	case *kind.MultipleOf:
		return "must be a multiple of " + ratString(k.Want)
	case *kind.MinLength:
		return fmt.Sprintf("must be at least %d characters long", k.Want)
	case *kind.MaxLength:
		return fmt.Sprintf("must be at most %d characters long", k.Want)
	case *kind.MinItems:
		return fmt.Sprintf("must have at least %d items", k.Want)
	case *kind.MaxItems:
		return fmt.Sprintf("must have at most %d items", k.Want)
	case *kind.MinProperties:
		return fmt.Sprintf("must have at least %d properties", k.Want)
	case *kind.MaxProperties:
		return fmt.Sprintf("must have at most %d properties", k.Want)
	case *kind.UniqueItems:
		return "items must be unique"
	case *kind.Required:
		// the missing properties are declared by the schema
		missing := slices.Sorted(slices.Values(k.Missing))
		return "missing required properties " + strings.Join(missing, ", ")
	case *kind.AdditionalProperties:
		return "contains unknown properties"
	case *kind.PropertyNames:
		return "contains an invalid property name"
	case *kind.FalseSchema:
		return "is not allowed"
	}
	if errorKind != nil {
		if keywordPath := errorKind.KeywordPath(); len(keywordPath) > 0 {
			return fmt.Sprintf("does not satisfy '%s'", strings.Join(keywordPath, "/"))
		}
	}
	return "is invalid"
}

// joinOr joins values as "a", "a or b", "a, b or c"
func joinOr(values []string) string {
	if len(values) <= 1 {
		return strings.Join(values, "")
	}
	return strings.Join(values[:len(values)-1], ", ") + " or " + values[len(values)-1]
}

func schemaValue(value any) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprint(value)
}

func ratString(value *big.Rat) string {
	if value == nil {
		return "?"
	}
	f, _ := value.Float64()
	return strconv.FormatFloat(f, 'g', -1, 64)
}
