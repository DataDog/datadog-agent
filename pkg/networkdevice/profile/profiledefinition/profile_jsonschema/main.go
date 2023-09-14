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

	"github.com/invopop/jsonschema"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

func generateBackendJSON(output string) error {
	schema := jsonschema.Reflect(&profiledefinition.DeviceProfileRcConfig{})
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}
	schemaJSON = append(schemaJSON, byte('\n'))
	return os.WriteFile(output, schemaJSON, 0664)
}

func main() {
	var (
		output string
	)

	flag.StringVar(&output, "output", "./device_profile_rc_config_schema.json", "Backend JSON schema generated file")
	flag.Parse()

	if err := generateBackendJSON(output); err != nil {
		panic(err)
	}
}
