// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/xeipuuv/gojsonschema"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/schemas"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

func upstreamEventSchemaURL(ref string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/DataDog/datadog-agent/%s/docs/cloud-workload-security/backend_linux.schema.json", ref)
}

// resolveUpstreamEventSchema returns the public schema URL pinned to the common ancestor with main. When that
// commit doesn't exist on the public repository (e.g. a mirror whose history has internal-only commits), it
// falls back to the schema at the tip of the public main branch instead of failing every test with a 404.
var resolveUpstreamEventSchema = sync.OnceValue(func() string {
	pinned := upstreamEventSchemaURL(GitAncestorOnMain)
	if GitAncestorOnMain == "main" {
		return pinned
	}

	// retry the probe so that a transient network error doesn't pin us to a URL that may not exist
	client := &http.Client{Timeout: 10 * time.Second}
	for attempt := 0; attempt < 5; attempt++ {
		resp, err := client.Head(pinned)
		if err != nil {
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			return pinned
		}
		fallback := upstreamEventSchemaURL("main")
		fmt.Printf("Upstream schema not found at %s, falling back to %s\n", pinned, fallback)
		return fallback
	}
	return pinned // still unreachable: let validateURLSchema retry and report the error
})

//nolint:unused
func validateMessageSchema(t *testing.T, msg string) bool {
	t.Helper()
	if !validateStringSchema(t, msg, "file:///message.schema.json") {
		return false
	}
	return validateURLSchema(t, msg, resolveUpstreamEventSchema())
}

//nolint:unused
func (tm *testModule) validateEventSchema(t *testing.T, event *model.Event, path string) bool {
	t.Helper()

	eventJSON, err := tm.marshalEvent(event)
	if err != nil {
		t.Error(err)
		return false
	}

	return validateStringSchema(t, eventJSON, path)
}

//nolint:unused
func (tm *testModule) validateExecSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///exec.schema.json")
}

//nolint:unused
func (tm *testModule) validateExitSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///exit.schema.json")
}

//nolint:unused
func (tm *testModule) validateOpenSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///open.schema.json")
}

//nolint:unused
func (tm *testModule) validateRenameSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///rename.schema.json")
}

//nolint:unused
func (tm *testModule) validateChmodSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///chmod.schema.json")
}

//nolint:unused
func (tm *testModule) validateChownSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///chown.schema.json")
}

//nolint:unused
func (tm *testModule) validateSELinuxSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///selinux.schema.json")
}

//nolint:unused
func (tm *testModule) validateLinkSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///link.schema.json")
}

//nolint:unused
func (tm *testModule) validateSpanSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///span.schema.json")
}

//nolint:unused
func (tm *testModule) validateUserSessionSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///user_session.schema.json")
}

//nolint:unused
func (tm *testModule) validateBPFSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///bpf.schema.json")
}

//nolint:unused
func (tm *testModule) validateMMapSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///mmap.schema.json")
}

//nolint:unused
func (tm *testModule) validateMProtectSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///mprotect.schema.json")
}

//nolint:unused
func (tm *testModule) validatePTraceSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///ptrace.schema.json")
}

//nolint:unused
func (tm *testModule) validateSetrlimitSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///setrlimit.schema.json")
}

//nolint:unused
func (tm *testModule) validateUnshareSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///unshare.schema.json")
}

//nolint:unused
func (tm *testModule) validateSetNSSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///setns.schema.json")
}

//nolint:unused
func (tm *testModule) validateLoadModuleSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///load_module.schema.json")
}

//nolint:unused
func (tm *testModule) validateLoadModuleNoFileSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///load_module_no_file.schema.json")
}

//nolint:unused
func (tm *testModule) validateUnloadModuleSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///unload_module.schema.json")
}

//nolint:unused
func (tm *testModule) validateSignalSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///signal.schema.json")
}

//nolint:unused
func (tm *testModule) validateSpliceSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///splice.schema.json")
}

//nolint:unused
func (tm *testModule) validateDNSSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///dns.schema.json")
}

//nolint:unused
func (tm *testModule) validateIMDSSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///imds.schema.json")
}

//nolint:unused
func (tm *testModule) validateSysctlSchema(t *testing.T, event *model.Event) bool {
	t.Helper()
	return tm.validateEventSchema(t, event, "file:///sysctl.schema.json")
}

//nolint:unused
func (tm *testModule) validateAcceptSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///accept.schema.json")
}

//nolint:unused
func (tm *testModule) validateBindSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///bind.schema.json")
}

//nolint:unused
func (tm *testModule) validateConnectSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///connect.schema.json")
}

//nolint:unused
func (tm *testModule) validateSocketSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///socket.schema.json")
}

//nolint:unused
func (tm *testModule) validateMountSchema(t *testing.T, event *model.Event) bool {
	if ebpfLessEnabled {
		return true
	}

	t.Helper()
	return tm.validateEventSchema(t, event, "file:///mount.schema.json")
}

//nolint:unused
func validateRuleSetLoadedSchema(t *testing.T, event *events.CustomEvent) bool {
	t.Helper()

	eventJSON, err := serializers.MarshalCustomEvent(event)
	if err != nil {
		t.Error(err)
		return false
	}

	return validateStringSchema(t, string(eventJSON), "file:///ruleset_loaded.schema.json")
}

//nolint:unused
func validateRawPacketActionSchema(t *testing.T, msg string) bool {
	t.Helper()

	return validateStringSchema(t, msg, "file:///rawpacket_action.schema.json")
}

//nolint:unused
func validateHeartbeatSchema(t *testing.T, event *events.CustomEvent) bool {
	t.Helper()

	eventJSON, err := serializers.MarshalCustomEvent(event)
	if err != nil {
		t.Error(err)
		return false
	}

	return validateStringSchema(t, string(eventJSON), "file:///heartbeat.schema.json")
}

// ValidInodeFormatChecker defines the format inode checker
//
//nolint:unused
type ValidInodeFormatChecker struct{}

// IsFormat check inode format
//
//nolint:unused
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
	return !model.IsFakeInode(inode)
}

func validateSchema(t *testing.T, schemaLoader gojsonschema.JSONLoader, documentLoader gojsonschema.JSONLoader) (bool, error) {
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return false, err
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
	return success, nil
}

//nolint:unused
func validateStringSchema(t *testing.T, json string, path string) bool {
	t.Helper()

	fs := http.FS(schemas.AssetFS)
	gojsonschema.FormatCheckers.Add("ValidInode", ValidInodeFormatChecker{})

	documentLoader := gojsonschema.NewStringLoader(json)
	schemaLoader := gojsonschema.NewReferenceLoaderFileSystem(path, fs)

	valid, err := validateSchema(t, schemaLoader, documentLoader)
	if err != nil {
		t.Error(err)
		return false
	}

	if !valid {
		t.Error(json)
		return false
	}

	return true
}

//nolint:unused
func validateURLSchema(t *testing.T, json string, url string) bool {
	t.Helper()

	documentLoader := gojsonschema.NewStringLoader(json)
	schemaLoader := gojsonschema.NewReferenceLoader(url)

	valid, err := backoff.Retry(t.Context(), func() (bool, error) {
		valid, err := validateSchema(t, schemaLoader, documentLoader)
		if err != nil && !errors.Is(err, syscall.ECONNRESET) {
			return valid, backoff.Permanent(err)
		}
		return valid, err
	}, backoff.WithMaxElapsedTime(1*time.Minute))
	if err != nil {
		t.Error(err)
		return false
	}

	if !valid {
		t.Error(json)
		return false
	}

	return true
}
