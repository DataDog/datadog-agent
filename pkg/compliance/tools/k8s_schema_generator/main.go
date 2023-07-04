// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/DataDog/datadog-agent/pkg/compliance/k8sconfig"
	"github.com/invopop/jsonschema"
)

// This tool produces a JSON schema of the k8sconfig types hierarchy.
// go run ./pkg/compliance/tools/k8s_schema_generator/main.go
func main() {
	reflector := &jsonschema.Reflector{
		RequiredFromJSONSchemaTags: true,
	}
	schema := reflector.Reflect(&k8sconfig.K8sNodeConfig{})
	b, err := json.MarshalIndent(schema, "", "    ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}
