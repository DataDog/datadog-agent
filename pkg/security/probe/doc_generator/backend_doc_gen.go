// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors/doc"
	"github.com/alecthomas/jsonschema"
)

func generateBackendJSON(output string) error {
	reflector := jsonschema.Reflector{
		ExpandedStruct: true,
		DoNotReference: false,
		TypeNamer:      doc.JSONTypeMapper,
	}
	schema := reflector.Reflect(&probe.EventSerializer{})
	schemaJSON, err := schema.MarshalJSON()
	if err != nil {
		return err
	}

	var out bytes.Buffer
	if err := json.Indent(&out, schemaJSON, "", "  "); err != nil {
		return err
	}

	return ioutil.WriteFile(output, out.Bytes(), 0664)
}

func main() {
	var (
		output string
	)

	flag.StringVar(&output, "output", "", "Backend JSON schema generated file")
	flag.Parse()

	if err := generateBackendJSON(output); err != nil {
		panic(err)
	}
}
