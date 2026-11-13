// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package schema

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/invopop/jsonschema"
	"maps"
	"reflect"
	"strings"
)

var (
	listMapPkgPath     = reflect.TypeFor[profiledefinition.ListMap[string]]().PkgPath()
	metadataConfigType = reflect.TypeFor[profiledefinition.MetadataConfig]()
)

// Reflect reflects v, defining each ListMap type once and referencing it, the way invopop handles types
// providing their own schema. The schema logic lives here so that profiledefinition doesn't link invopop.
func Reflect(v any) *jsonschema.Schema {
	listMaps := jsonschema.Definitions{}
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		Mapper: func(t reflect.Type) *jsonschema.Schema {
			if !isListMap(t) {
				return nil
			}
			listMaps[t.Name()] = listMapSchema(t)
			return &jsonschema.Schema{Ref: "#/$defs/" + t.Name()}
		},
	}
	schema := reflector.Reflect(v)
	maps.Copy(schema.Definitions, listMaps)
	return schema
}

// isListMap reports whether t is a profiledefinition.ListMap, or a type defined from one.
func isListMap(t reflect.Type) bool {
	return t == metadataConfigType ||
		t.Kind() == reflect.Map && t.PkgPath() == listMapPkgPath && strings.HasPrefix(t.Name(), "ListMap[")
}

// listMapSchema is needed to customize jsonschema to match []MapItem[T] used in json format
func listMapSchema(t reflect.Type) *jsonschema.Schema {
	item := reflect.StructOf([]reflect.StructField{
		{Name: "Key", Type: reflect.TypeFor[string](), Tag: `json:"key"`},
		{Name: "Value", Type: t.Elem(), Tag: `json:"value"`},
	})
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
		Mapper: func(t reflect.Type) *jsonschema.Schema {
			if !isListMap(t) {
				return nil
			}
			return listMapSchema(t)
		},
	}
	schema := reflector.ReflectFromType(reflect.SliceOf(item))
	// don't need version because this is a child of a versioned schema.
	schema.Version = ""
	return schema
}
