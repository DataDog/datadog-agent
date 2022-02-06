// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// Module represents everything needed to generate the accessors for a specific module (fields, build tags, ...)
type Module struct {
	Name            string
	SourcePkgPrefix string
	SourcePkg       string
	TargetPkg       string
	BuildTags       []string
	Fields          map[string]*StructField
	Iterators       map[string]*StructField
	EventTypes      map[string]bool
	EventTypeDocs   map[string]string
}

// StructField represents a structure field for which an accessor will be generated
type StructField struct {
	Name          string
	Prefix        string
	Struct        string
	BasicType     string
	ReturnType    string
	IsArray       bool
	Event         string
	Handler       string
	OrigType      string
	IsOrigTypePtr bool
	Iterator      *StructField
	Weight        int64
	CommentText   string
	OpOverrides   string
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
