// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common holds common related files
package common

import "fmt"

// EventTypeMetadata is used to iterate over the model from the event types
type EventTypeMetadata struct {
	Doc    string
	Fields []string
}

// NewEventTypeMetada returns a new EventTypeMetada
func NewEventTypeMetada(fields ...string) *EventTypeMetadata {
	return &EventTypeMetadata{
		Fields: fields,
	}
}

// Platform defines which platform a structure is associated with
type Platform string

const (
	Unspecified Platform = "unspecified" // Unspecified defines unspecified platforms
	Linux       Platform = "linux"       // Linux defines linux platforms
	Windows     Platform = "windows"     // Windows defines windows platforms
)

// Module represents everything needed to generate the accessors for a specific module (fields, build tags, ...)
type Module struct {
	Name            string
	SourcePkgPrefix string
	SourcePkg       string
	TargetPkg       string
	BuildTags       []string
	Fields          map[string]*StructField // Fields only contains fields that are exposed in SECL
	//GettersOnlyFields map[string]*StructField // GettersOnlyFields only contains fields that have generated getters but are not exposed in SECL
	AllFields  map[string]*StructField
	Iterators  map[string]*StructField
	EventTypes map[string]*EventTypeMetadata
	Mock       bool
	Imports    []string
}

// StructField represents a structure field for which an accessor will be generated
type StructField struct {
	Name             string
	Prefix           string
	Struct           string
	BasicType        string
	ReturnType       string
	IsArray          bool
	IsLength         bool
	Event            string
	Handler          string
	Helper           bool // specify the handler as just a helper and not a real resolver. It means that this handler won't be called by the ResolveFields function
	SkipADResolution bool
	OrigType         string
	IsOrigTypePtr    bool
	Iterator         *StructField
	Weight           int64
	CommentText      string
	OpOverrides      string
	Check            string
	Alias            string
	AliasPrefix      string
	GettersOnly      bool
	Ref              string
	RestrictedTo     []string
	IsIterator       bool
}

// GetEvaluatorType returns the evaluator type name
func (sf *StructField) GetEvaluatorType() string {
	var evaluatorType string
	if sf.IsLength && sf.IsIterator {
		evaluatorType = "eval.IntEvaluator"
	} else if sf.ReturnType == "int" {
		evaluatorType = "eval.IntEvaluator"
		if sf.Iterator != nil || sf.IsArray {
			evaluatorType = "eval.IntArrayEvaluator"
		}
	} else if sf.ReturnType == "bool" {
		evaluatorType = "eval.BoolEvaluator"
		if sf.Iterator != nil || sf.IsArray {
			evaluatorType = "eval.BoolArrayEvaluator"
		}
	} else if sf.ReturnType == "net.IPNet" {
		evaluatorType = "eval.CIDREvaluator"
		if sf.IsArray {
			evaluatorType = "eval.CIDRValuesEvaluator"
		}
	} else {
		evaluatorType = "eval.StringEvaluator"
		if sf.Iterator != nil || sf.IsArray {
			evaluatorType = "eval.StringArrayEvaluator"
		}
	}
	return evaluatorType
}

// GetDefaultReturnValue returns default value for the given return type
func (sf *StructField) GetDefaultReturnValue() string {
	if sf.ReturnType == "int" {
		if sf.Iterator != nil || sf.IsArray {
			return "[]int{}"
		}
		return "0"
	} else if sf.ReturnType == "bool" {
		if sf.Iterator != nil || sf.IsArray {
			return "[]bool{}"
		}
		return "false"
	} else if sf.ReturnType == "net.IPNet" {
		if sf.IsArray {
			return "&eval.CIDRValues{}"
		}
		return "net.IPNet{}"
	} else if sf.Iterator != nil || sf.IsArray {
		return "[]string{}"
	}
	return `""`
}

// GetDefaultScalarReturnValue returns default scalar value for the given return type
func (sf *StructField) GetDefaultScalarReturnValue() string {
	if sf.ReturnType == "int" {
		return "0"
	} else if sf.ReturnType == "bool" {
		return "false"
	} else if sf.ReturnType == "net.IPNet" {
		return "net.IPNet{}"
	}
	return `""`
}

// GetArrayPrefix returns the array prefix of this field
func (sf *StructField) GetArrayPrefix() string {
	if sf.IsArray {
		return "[]"
	}
	return ""
}

// GetCacheName returns the cache name
func (sf *StructField) GetCacheName() string {
	switch sf.ReturnType {
	case "string":
		return "StringCache"
	case "int":
		return "IntCache"
	case "bool":
		return "BoolCache"
	default:
		panic(fmt.Sprintf("no cache name defined for return type '%s'", sf.ReturnType))
	}
}
