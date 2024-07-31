// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/schemas/policy -output ../../../tests/schemas/policy.schema.json

// Package main holds main related files
package main

import (
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"time"

	"github.com/invopop/jsonschema"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func main() {
	var output string
	flag.StringVar(&output, "output", "", "output file")
	flag.Parse()

	if output == "" {
		panic("an output file argument is required")
	}

	reflector := jsonschema.Reflector{
		ExpandedStruct: true,
		Mapper: func(t reflect.Type) *jsonschema.Schema {
			switch t {
			case reflect.TypeOf(time.Duration(0)):
				return &jsonschema.Schema{
					Type:   "string",
					Format: "duration",
				}
			}
			return nil
		},
	}

	if err := reflector.AddGoComments("github.com/DataDog/datadog-agent/pkg/security/secl/rules/model.go", "../../../secl/rules"); err != nil {
		panic(err)
	}

	schema := reflector.Reflect(&rules.PolicyDef{})
	schema.ID = "https://github.com/DataDog/datadog-agent/tree/main/pkg/security/secl/rules"

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(output, data, 0644); err != nil {
		panic(err)
	}
}
