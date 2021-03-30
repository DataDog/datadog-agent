// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build functionaltests

package tests

import (
	"testing"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/xeipuuv/gojsonschema"
)

func validExecSchema(t *testing.T, event *sprobe.Event) {
	schemaLoader := gojsonschema.NewReferenceLoader("file://pkg/security/tests/schemas/exec.schema.json")
	documentLoader := gojsonschema.NewReferenceLoader("file://pkg/security/tests/schemas/doc.json")

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Errorf("%s", desc)
		}
	}
}
