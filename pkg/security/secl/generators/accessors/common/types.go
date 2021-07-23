// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

type Module struct {
	Name            string
	SourcePkgPrefix string
	SourcePkg       string
	TargetPkg       string
	BuildTags       []string
	Fields          map[string]*StructField
	Iterators       map[string]*StructField
	EventTypes      map[string]bool
	Mock            bool
}

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
}
