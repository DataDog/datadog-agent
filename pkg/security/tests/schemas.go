// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && (functionaltests || stresstests)

// Package tests holds tests related files
package tests

import (
	"embed"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"testing"

	"github.com/xeipuuv/gojsonschema"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

func getUpstreamEventSchema() string {
	sha, _ := os.LookupEnv("CI_COMMIT_SHA")
	if sha == "" {
		sha = "main"
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/DataDog/datadog-agent/%s/docs/cloud-workload-security/backend_linux.schema.json", sha)
}

var upstreamEventSchema = getUpstreamEventSchema()

//nolint:deadcode,unused
//go:embed schemas
var schemaAssetFS embed.FS

// ValidInodeFormatChecker defines the format inode checker
//
//nolint:deadcode,unused
type ValidInodeFormatChecker struct{}

// IsFormat check inode format
//
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
	return !dentry.IsFakeInode(inode)
}

func validateSchema(t *testing.T, schemaLoader gojsonschema.JSONLoader, documentLoader gojsonschema.JSONLoader) bool {
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		t.Error(err)
		return false
	}

	success := true

	if !result.Valid() {
		for _, err := range result.Errors() {
			// allow addition properties
			if err.Type() == "additional_property_not_allowed" {
				continue
			}

			t.Error(err)
			success = false
		}
	}
	return success
}

//nolint:deadcode,unused
func validateStringSchema(t *testing.T, json string, path string) bool {
	t.Helper()

	fs := http.FS(schemaAssetFS)
	gojsonschema.FormatCheckers.Add("ValidInode", ValidInodeFormatChecker{})

	documentLoader := gojsonschema.NewStringLoader(json)
	schemaLoader := gojsonschema.NewReferenceLoaderFileSystem(path, fs)

	if !validateSchema(t, schemaLoader, documentLoader) {
		t.Error(json)
		return false
	}

	return true
}

//nolint:deadcode,unused
func validateUrlSchema(t *testing.T, json string, url string) bool {
	t.Helper()

	documentLoader := gojsonschema.NewStringLoader(json)
	schemaLoader := gojsonschema.NewReferenceLoader(url)

	if !validateSchema(t, schemaLoader, documentLoader) {
		t.Error(json)
		return false
	}

	return true
}

//nolint:deadcode,unused
func (tm *testModule) validateEventSchema(t *testing.T, event *model.Event, path string) bool {
	t.Helper()

	eventJSON, err := tm.marshalEvent(event)
	if err != nil {
		t.Error(err)
		return false
	}

	return validateStringSchema(t, eventJSON, path)
}

//nolint:deadcode,unused
func (tm *testModule) validateExecSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/exec.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateExitSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/exit.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateOpenSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/open.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateRenameSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/rename.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateChmodSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/chmod.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateChownSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/chown.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateSELinuxSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/selinux.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateLinkSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/link.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateSpanSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/span.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateUserSessionSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/user_session.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateBPFSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/bpf.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateMMapSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/mmap.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateMProtectSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/mprotect.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validatePTraceSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/ptrace.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateLoadModuleSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/load_module.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateLoadModuleNoFileSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/load_module_no_file.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateUnloadModuleSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/unload_module.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateSignalSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/signal.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateSpliceSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/splice.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateDNSSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/dns.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateIMDSSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/imds.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateBindSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/bind.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateConnectSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/connect.schema.json")
}

//nolint:deadcode,unused
func (tm *testModule) validateMountSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///schemas/mount.schema.json")
}

//nolint:deadcode,unused
func validateRuleSetLoadedSchema(t *testing.T, event *events.CustomEvent) bool {
	t.Helper()

	eventJSON, err := serializers.MarshalCustomEvent(event)
	if err != nil {
		t.Error(err)
		return false
	}

	return validateStringSchema(t, string(eventJSON), "file:///schemas/ruleset_loaded.schema.json")
}

//nolint:deadcode,unused
func validateHeartbeatSchema(t *testing.T, event *events.CustomEvent) bool {
	t.Helper()

	eventJSON, err := serializers.MarshalCustomEvent(event)
	if err != nil {
		t.Error(err)
		return false
	}

	return validateStringSchema(t, string(eventJSON), "file:///schemas/heartbeat.schema.json")
}

//nolint:deadcode,unused
func validateActivityDumpProtoSchema(t *testing.T, ad string) bool {
	t.Helper()
	return validateStringSchema(t, ad, "file:///schemas/activity_dump_proto.schema.json")
}

//nolint:deadcode,unused
func validateMessageSchema(t *testing.T, msg string) bool {
	t.Helper()
	if !validateStringSchema(t, msg, "file:///schemas/message.schema.json") {
		return false
	}
	return validateUrlSchema(t, msg, upstreamEventSchema)
}
