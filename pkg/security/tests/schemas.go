// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests
// +build functionaltests stresstests

package tests

import (
	"embed"
	"math/big"
	"net/http"
	"testing"

	"github.com/xeipuuv/gojsonschema"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
)

//nolint:deadcode,unused
//go:embed schemas
var schemaAssetFS embed.FS

// ValidInodeFormatChecker defines the format inode checker
//nolint:deadcode,unused
type ValidInodeFormatChecker struct{}

// IsFormat check inode format
//nolint:deadcode,unused
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

//nolint:deadcode,unused
func validateSchema(t *testing.T, event *sprobe.Event, path string) bool {
	fs := http.FS(schemaAssetFS)

	gojsonschema.FormatCheckers.Add("ValidInode", ValidInodeFormatChecker{})

	serialized := event.String()

	documentLoader := gojsonschema.NewStringLoader(serialized)
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

//nolint:deadcode,unused
func validateExecSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/exec.schema.json")
}

//nolint:deadcode,unused
func validateOpenSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/open.schema.json")
}

//nolint:deadcode,unused
func validateRenameSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/rename.schema.json")
}

//nolint:deadcode,unused
func validateChmodSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/chmod.schema.json")
}

//nolint:deadcode,unused
func validateChownSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/chown.schema.json")
}

//nolint:deadcode,unused
func validateSELinuxSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/selinux.schema.json")
}

//nolint:deadcode,unused
func validateLinkSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/link.schema.json")
}

//nolint:deadcode,unused
func validateSpanSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/span.schema.json")
}

//nolint:deadcode,unused
func validateBPFSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/bpf.schema.json")
}

//nolint:deadcode,unused
func validateMMapSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/mmap.schema.json")
}

//nolint:deadcode,unused
func validateMProtectSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/mprotect.schema.json")
}

//nolint:deadcode,unused
func validatePTraceSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/ptrace.schema.json")
}

//nolint:deadcode,unused
func validateLoadModuleSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/load_module.schema.json")
}

//nolint:deadcode,unused
func validateLoadModuleNoFileSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/load_module_no_file.schema.json")
}

//nolint:deadcode,unused
func validateUnloadModuleSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/unload_module.schema.json")
}

//nolint:deadcode,unused
func validateSignalSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/signal.schema.json")
}

//nolint:deadcode,unused
func validateSpliceSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/splice.schema.json")
}

//nolint:deadcode,unused
func validateDNSSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/dns.schema.json")
}

//nolint:deadcode,unused
func validateBindSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///schemas/bind.schema.json")
}
