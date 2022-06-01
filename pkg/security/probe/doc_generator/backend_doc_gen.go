// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package main

import (
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/alecthomas/jsonschema"
)

func generateBackendJSON(output string) error {
	reflector := jsonschema.Reflector{
		ExpandedStruct: true,
		DoNotReference: false,
		TypeMapper:     jsonTypeMapper,
		TypeNamer:      jsonTypeNamer,
	}
	schema := reflector.Reflect(&probe.EventSerializer{})

	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(output, schemaJSON, 0664)
}

func jsonTypeMapper(ty reflect.Type) *jsonschema.Type {
	if ty == reflect.TypeOf(utils.EasyjsonTime{}) {
		return jsonschema.Reflect(time.Time{}).Type
	}
	return nil
}

func jsonTypeNamer(ty reflect.Type) string {
	const selinuxPrefix = "selinux"

	base := strings.TrimSuffix(ty.Name(), "Serializer")
	if strings.HasPrefix(base, selinuxPrefix) {
		return "SELinux" + strings.TrimPrefix(base, selinuxPrefix)
	}

	return base
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
