// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build functionaltests

package tests

import (
	"embed"
	"math/big"
	"net/http"
	"testing"

	"github.com/xeipuuv/gojsonschema"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
)

//go:embed schemas
var schemaAssetFS embed.FS

// ValidInodeFormatChecker defines the format inode checker
type ValidInodeFormatChecker struct{}

// IsFormat check inode format
func (v ValidInodeFormatChecker) IsFormat(input interface{}) bool {

	var inode uint64
	switch t := input.(type) {
	case float64:
		inode = uint64(t)
	case *big.Int:
		inode = t.Uint64()
	case *big.Float:
		inode, _ = t.Uint64()
	case *big.Rat:
		f, _ := t.Float64()
		inode = uint64(f)
	default:
		return false
	}
	return !sprobe.IsFakeInode(inode)
}

func validateSchema(t *testing.T, event *sprobe.Event, path string) bool {
	fs := http.FS(schemaAssetFS)

	gojsonschema.FormatCheckers.Add("ValidInode", ValidInodeFormatChecker{})

	documentLoader := gojsonschema.NewStringLoader(event.String())
	schemaLoader := gojsonschema.NewReferenceLoaderFileSystem(path, fs)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		t.Error(err)
		return false
	}

	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Errorf("%s", desc)
		}
		return false
	}

	return true
}

func validateExecSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/exec.schema.json")
}

func validateOpenSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/open.schema.json")
}

func validateRenameSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/rename.schema.json")
}

func validateChmodSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/chmod.schema.json")
}

func validateChownSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/chown.schema.json")
}

func validateSELinuxSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/selinux.schema.json")
}

func validateLinkSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/link.schema.json")
}

func validateSpanSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/span.schema.json")
}

func validateBPFSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/bpf.schema.json")
}
