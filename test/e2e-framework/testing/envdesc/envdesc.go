// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package envdesc provides functions to serialize and deserialize provisioned
// E2E environments to/from a JSON descriptor, enabling the provision-then-install
// split used by the e2e-install CLI and manual QA tasks.
//
// The descriptor format is identical to the Pulumi stack output format used
// internally by the test framework: a map[string][]byte where each key is a
// component name and the value is the JSON of that component's output struct.
// This means a Pulumi stack's outputs (obtained via `pulumi stack output --json
// --show-secrets`) can be used directly as an envdesc descriptor.
package envdesc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

const importKey = "import"

// Descriptor is a serialized E2E environment. It wraps the RawResources map
// (component name → component output JSON) together with the scenario name
// and environment type for informational purposes.
type Descriptor struct {
	// Scenario is the scenario registry name (e.g. "aws/vm", "aws/eks").
	Scenario string `json:"scenario"`
	// EnvType identifies the environment struct ("host", "kubernetes", "dockerhost", "windowshost", "ecs").
	EnvType string `json:"env_type"`
	// Resources maps each component's import key to its JSON-serialized output.
	// This is the same format as provisioners.RawResources / Pulumi stack outputs.
	Resources map[string]json.RawMessage `json:"resources"`
}

// WriteToFile serializes d to a JSON file at path.
func WriteToFile(d *Descriptor, path string) error {
	data, err := json.MarshalIndent(d, "", "\t")
	if err != nil {
		return fmt.Errorf("envdesc.WriteToFile: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// ReadFromFile deserializes a Descriptor from a JSON file at path.
func ReadFromFile(path string) (*Descriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("envdesc.ReadFromFile: %w", err)
	}
	var d Descriptor
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("envdesc.ReadFromFile: %w", err)
	}
	return &d, nil
}

// FromRawResources wraps a provisioners.RawResources map into a Descriptor.
// Use this to dump the env immediately after Pulumi finishes.
func FromRawResources(scenario, envType string, resources provisioners.RawResources) *Descriptor {
	d := &Descriptor{
		Scenario:  scenario,
		EnvType:   envType,
		Resources: make(map[string]json.RawMessage, len(resources)),
	}
	for k, v := range resources {
		d.Resources[k] = json.RawMessage(v)
	}
	return d
}

// DumpEnv serializes the Importable fields of a populated env struct into a
// Descriptor. Use this after a Pulumi run to capture the provisioned state.
func DumpEnv[Env any](scenario, envType string, env *Env) (*Descriptor, error) {
	d := &Descriptor{
		Scenario:  scenario,
		EnvType:   envType,
		Resources: make(map[string]json.RawMessage),
	}

	envFields := reflect.VisibleFields(reflect.TypeOf(env).Elem())
	envValue := reflect.ValueOf(env).Elem()

	for _, field := range envFields {
		if !field.IsExported() {
			continue
		}
		fv := envValue.FieldByIndex(field.Index)
		if fv.IsNil() {
			continue
		}
		imp, ok := fv.Interface().(components.Importable)
		if !ok {
			continue
		}

		key := field.Tag.Get(importKey)
		if key == "" {
			key = imp.Key()
		}
		if key == "" {
			key = field.Name
		}

		data, err := json.Marshal(fv.Interface())
		if err != nil {
			return nil, fmt.Errorf("envdesc.DumpEnv: field %s: %w", field.Name, err)
		}
		d.Resources[key] = json.RawMessage(data)
	}

	return d, nil
}

// WriteEnvToFile is a convenience wrapper: DumpEnv + WriteToFile.
func WriteEnvToFile[Env any](scenario, envType string, env *Env, path string) error {
	d, err := DumpEnv(scenario, envType, env)
	if err != nil {
		return err
	}
	return WriteToFile(d, path)
}

// WriteEnvToDirectory writes each component as a separate JSON file inside
// dir (one file per component, filename = import key + ".json"). This format
// is compatible with provisioners.FileProvisioner.
func WriteEnvToDirectory[Env any](scenario, envType string, env *Env, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	d, err := DumpEnv(scenario, envType, env)
	if err != nil {
		return err
	}
	// Write descriptor metadata
	meta := &Descriptor{Scenario: scenario, EnvType: envType}
	metaData, _ := json.MarshalIndent(meta, "", "\t")
	if err := os.WriteFile(filepath.Join(dir, "_descriptor.json"), metaData, 0o600); err != nil {
		return err
	}
	for k, v := range d.Resources {
		if err := os.WriteFile(filepath.Join(dir, k+".json"), v, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// toRawResources converts the Descriptor's Resources to the provisioners.RawResources type.
func (d *Descriptor) toRawResources() provisioners.RawResources {
	rr := make(provisioners.RawResources, len(d.Resources))
	for k, v := range d.Resources {
		rr[k] = []byte(v)
	}
	return rr
}

// LoadEnv deserializes a Descriptor into a populated, Init-ed environment.
// ctx is used for component Init calls (SSH client setup, k8s client setup etc).
// Returns the populated env, ready for use by installer functions.
func LoadEnv[Env any](d *Descriptor, ctx common.Context) (*Env, error) {
	env, fields, values, err := environments.CreateEnv[Env]()
	if err != nil {
		return nil, fmt.Errorf("envdesc.LoadEnv: %w", err)
	}

	resources := d.toRawResources()
	if err := buildEnvFromResources(resources, fields, values, ctx); err != nil {
		return nil, fmt.Errorf("envdesc.LoadEnv: %w", err)
	}

	if initializable, ok := any(env).(common.Initializable); ok {
		if err := initializable.Init(ctx); err != nil {
			return nil, fmt.Errorf("envdesc.LoadEnv: Init: %w", err)
		}
	}

	return env, nil
}

// buildEnvFromResources populates the given env field values from the RawResources
// map. Key lookup order:
//  1. `import:` struct tag on the field (same as Pulumi path)
//  2. importable.Key() (set by Export, available when loading from Pulumi outputs)
//  3. Field name (fallback; used when loading from a descriptor written by DumpEnv)
//
// Missing optional resources (Agent, Updater) are silently skipped so that
// "infra-only" descriptors (no agent in the resources map) load cleanly.
func buildEnvFromResources(resources provisioners.RawResources, fields []reflect.StructField, values []reflect.Value, ctx common.Context) error {
	if len(fields) != len(values) {
		return fmt.Errorf("fields and values length mismatch")
	}
	if len(resources) == 0 {
		return nil
	}

	for idx, fieldValue := range values {
		field := fields[idx]
		importKeyFromTag := field.Tag.Get(importKey)

		if fieldValue.IsNil() {
			if importKeyFromTag != "" {
				if _, found := resources[importKeyFromTag]; found {
					return fmt.Errorf("resource %s has key %s but field is nil", field.Name, importKeyFromTag)
				}
			}
			continue
		}

		importable := fieldValue.Interface().(components.Importable)
		resourceKey := importable.Key()
		if importKeyFromTag != "" {
			resourceKey = importKeyFromTag
		}
		// Fallback to field name — used when loading from a descriptor written by
		// DumpEnv (which uses field names as keys, not Pulumi export names).
		if resourceKey == "" {
			resourceKey = field.Name
		}

		rawResource, found := resources[resourceKey]
		if !found {
			// Some fields (Agent, Updater) may legitimately be absent from an
			// infra-only descriptor. Mark them nil and continue rather than failing.
			fieldValue.Set(reflect.Zero(fieldValue.Type()))
			continue
		}

		if err := importable.Import(rawResource, fieldValue.Interface()); err != nil {
			return fmt.Errorf("failed to import resource %s (key %s): %w", field.Name, resourceKey, err)
		}

		if initializable, ok := fieldValue.Interface().(common.Initializable); ok {
			if err := initializable.Init(ctx); err != nil {
				return fmt.Errorf("failed to init resource %s (key %s): %w", field.Name, resourceKey, err)
			}
		}
	}

	return nil
}

// DetectEnvType inspects the keys of a Pulumi stack outputs map and returns the
// env_type string ("host", "kubernetes", "dockerhost", "ecs") by heuristic:
// - any value with "kubeConfig" key → kubernetes
// - any value with "ecsCluster" key → ecs
// - any value with "dockerManager" key → dockerhost
// - otherwise → host
func DetectEnvType(pulumiOutputs map[string]json.RawMessage) string {
	for _, v := range pulumiOutputs {
		var m map[string]json.RawMessage
		if json.Unmarshal(v, &m) != nil {
			continue
		}
		if _, ok := m["kubeConfig"]; ok {
			return "kubernetes"
		}
		if _, ok := m["ecsCluster"]; ok {
			return "ecs"
		}
		if _, ok := m["dockerManager"]; ok {
			return "dockerhost"
		}
	}
	return "host"
}

// MapPulumiOutputsToDescriptor converts raw Pulumi stack outputs (keyed by
// Pulumi export name, e.g. "dd-Host-aws-vm") into a Descriptor keyed by env
// field names ("RemoteHost", "FakeIntake", "KubernetesCluster", etc.) that
// envdesc.LoadEnv can read. Unknown outputs are silently skipped.
func MapPulumiOutputsToDescriptor(scenario string, pulumiOutputs map[string]json.RawMessage) *Descriptor {
	envType := DetectEnvType(pulumiOutputs)
	d := &Descriptor{
		Scenario:  scenario,
		EnvType:   envType,
		Resources: make(map[string]json.RawMessage),
	}

	fieldHints := map[string]string{
		// Fields that identify each env component type by a unique JSON key
		"address":       "RemoteHost",        // remote.HostOutput
		"kubeConfig":    "KubernetesCluster", // kubernetes.ClusterOutput
		"url":           "FakeIntake",        // fakeintake.FakeintakeOutput — check also for "host" field
		"dockerManager": "Docker",            // docker.ManagerOutput
	}
	usedFields := map[string]bool{}

	for _, v := range pulumiOutputs {
		var m map[string]json.RawMessage
		if json.Unmarshal(v, &m) != nil {
			continue
		}
		for jsonKey, fieldName := range fieldHints {
			if _, ok := m[jsonKey]; ok && !usedFields[fieldName] {
				// Extra disambiguation: "url" could be FakeIntake or something else.
				// FakeIntake has "url" + "host" + "port".
				if jsonKey == "url" {
					if _, hasHost := m["host"]; !hasHost {
						continue
					}
					if _, hasPort := m["port"]; !hasPort {
						continue
					}
				}
				d.Resources[fieldName] = v
				usedFields[fieldName] = true
				break
			}
		}
	}

	return d
}
