// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main holds main related files
package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/invopop/jsonschema"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// CWSEvent is a similar struct to what we actually send to the backend, except in the real case
// we actually do some struct-less string concatenation optimization
type CWSEvent struct {
	events.BackendEvent         `json:",inline"`
	serializers.EventSerializer `json:",inline"`
}

const serializersImportPath = "github.com/DataDog/datadog-agent/pkg/security/serializers"

func generateBackendJSON(output, serializersDir string) error {
	// AddGoComments keys its comment map on path.Join(importPath, dir-of-file),
	// so serializersDir has to be the working directory for the key to come out as
	// serializersImportPath. Resolve the output path before moving.
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(serializersDir); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(cwd) }()

	reflector := jsonschema.Reflector{
		ExpandedStruct: true,
		DoNotReference: false,
		Mapper:         jsonTypeMapper,
		Namer:          jsonTypeNamer,
	}

	if err := reflector.AddGoComments(serializersImportPath, "."); err != nil {
		return err
	}
	reflector.CommentMap = cleanupEasyjson(reflector.CommentMap)

	schema := reflector.Reflect(&CWSEvent{})
	schema.ID = "https://github.com/DataDog/datadog-agent/tree/main/pkg/security/serializers"

	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(absOutput, schemaJSON, 0664)
}

func jsonTypeMapper(ty reflect.Type) *jsonschema.Schema {
	if ty == reflect.TypeOf(utils.EasyjsonTime{}) {
		schema := jsonschema.Reflect(time.Time{})
		schema.Version = ""
		return schema
	}
	return nil
}

func cleanupEasyjson(commentMap map[string]string) map[string]string {
	res := make(map[string]string, len(commentMap))
	for name, comment := range commentMap {
		cleaned := strings.TrimSpace(comment)
		cleaned = strings.TrimSuffix(cleaned, "easyjson:json")
		res[name] = strings.TrimSpace(cleaned)
	}
	return res
}

func jsonTypeNamer(ty reflect.Type) string {
	const selinuxPrefix = "selinux"

	base := strings.TrimSuffix(ty.Name(), "Serializer")
	if after, ok := strings.CutPrefix(base, selinuxPrefix); ok {
		return "SELinux" + after
	}

	return base
}

func main() {
	var (
		output         string
		serializersDir string
	)

	flag.StringVar(&output, "output", "", "Backend JSON schema generated file")
	flag.StringVar(&serializersDir, "serializers-dir", ".", "Directory containing serializer .go source files")
	flag.Parse()

	if err := generateBackendJSON(output, serializersDir); err != nil {
		panic(err)
	}
}
