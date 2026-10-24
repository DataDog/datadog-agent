// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/schemas/policy -output ../../../secl/schemas/policy.schema.json

// Package main holds main related files
package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/invopop/jsonschema"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const rulesImportPath = "github.com/DataDog/datadog-agent/pkg/security/secl/rules"

func generatePolicyJSON(output, rulesDir string) error {
	// AddGoComments keys its comment map on path.Join(importPath, dir-of-file),
	// so rulesDir has to be the working directory for the key to come out as
	// rulesImportPath. Resolve the output path before moving.
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(rulesDir); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(cwd) }()

	reflector := jsonschema.Reflector{
		ExpandedStruct: true,
		Mapper: func(t reflect.Type) *jsonschema.Schema {
			switch t {
			case reflect.TypeOf(rules.HumanReadableDuration{}), reflect.TypeOf(time.Duration(0)):
				return &jsonschema.Schema{
					OneOf: []*jsonschema.Schema{
						{
							Type:        "string",
							Format:      "duration",
							Description: "Duration in Go format (e.g. 1h30m, see https://pkg.go.dev/time#ParseDuration)",
						},
						{
							Type:        "integer",
							Description: "Duration in nanoseconds",
						},
					},
				}
			}
			return nil
		},
	}

	if err := reflector.AddGoComments(rulesImportPath, "."); err != nil {
		return err
	}

	schema := reflector.Reflect(&rules.PolicyDef{})
	schema.ID = "https://github.com/DataDog/datadog-agent/tree/main/pkg/security/secl/rules"

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(absOutput, data, 0644)
}

func main() {
	var (
		output   string
		rulesDir string
	)

	flag.StringVar(&output, "output", "", "output file")
	flag.StringVar(&rulesDir, "rules-dir", "../../../secl/rules", "Directory containing secl/rules .go source files")
	flag.Parse()

	if output == "" {
		panic("an output file argument is required")
	}

	if err := generatePolicyJSON(output, rulesDir); err != nil {
		panic(err)
	}
}
