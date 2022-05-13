// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

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

// Module represents everything needed to generate the accessors for a specific module (fields, build tags, ...)
type Module struct {
	Name            string
	SourcePkgPrefix string
	SourcePkg       string
	TargetPkg       string
	BuildTags       []string
	Fields          map[string]*StructField // only exposed fields by SECL
	AllFields       map[string]*StructField
	Iterators       map[string]*StructField
	EventTypes      map[string]*EventTypeMetadata
	Mock            bool
}

// StructField represents a structure field for which an accessor will be generated
type StructField struct {
	Name                string
	Prefix              string
	Struct              string
	BasicType           string
	ReturnType          string
	IsArray             bool
	Event               string
	Handler             string
	CachelessResolution bool
	OrigType            string
	IsOrigTypePtr       bool
	Iterator            *StructField
	Weight              int64
	CommentText         string
	OpOverrides         string
}

// GetEvaluatorType returns the evaluator type name
func (sf *StructField) GetEvaluatorType() string {
	var evaluatorType string
	if sf.ReturnType == "int" {
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

// GetArrayPrefix returns the array prefix of this field
func (sf *StructField) GetArrayPrefix() string {
	if sf.IsArray {
		return "[]"
	}
	return ""
}
