// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioner

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
)

// SnapshotBindingsKey records canonical component keys (for example remoteHost)
// mapped to the actual resource keys exported by the provisioner. Resource names
// are preserved: two hosts must not be guessed apart using their payload shape.
const SnapshotBindingsKey = "_bindings"

// WriteSnapshotFileForEnv persists the resource bindings established by a typed
// provisioner as well as its resources. Raw Pulumi outputs alone cannot recreate
// these bindings: their resource names differ from the field names in a new Env.
// Call this while env still holds the keys assigned during provisioning.
func WriteSnapshotFileForEnv(path string, env any, resources RawResources, meta map[string]any) error {
	value, fields, err := snapshotFields(env)
	if err != nil {
		return err
	}
	bindings := make(map[string]string)
	for _, field := range fields {
		component, err := value.FieldByIndexErr(field.Index)
		if err != nil {
			return fmt.Errorf("reading snapshot component %s: %w", field.Name, err)
		}
		if component.IsNil() {
			continue // intentionally not provisioned (e.g. Agent before install)
		}
		key := component.Interface().(outputs.Importable).Key()
		if tag := field.Tag.Get("import"); tag != "" {
			key = tag // same precedence as environments.BuildEnvFromResources
		}
		if key == "" {
			return fmt.Errorf("snapshot component %s has no resource key", field.Name)
		}
		if _, ok := resources[key]; !ok {
			return fmt.Errorf("snapshot component %s references missing resource %q", field.Name, key)
		}
		canonical := snapshotFieldKey(field)
		if previous, ok := bindings[canonical]; ok && previous != key {
			return fmt.Errorf("snapshot components share key %q but reference different resources", canonical)
		}
		bindings[canonical] = key
	}

	metadata := maps.Clone(meta)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	delete(metadata, "bindings") // reserve the normalized metadata key for this exporter
	metadata[SnapshotBindingsKey] = bindings
	return WriteSnapshotFile(path, resources, metadata)
}

// UpdateSnapshotResource replaces a component under its canonical key. Update
// its binding too, so installing an Agent outside Pulumi does not keep pointing
// at an old Pulumi-exported Agent resource. All other resources and metadata are
// preserved; invalid bindings leave the original file untouched.
func UpdateSnapshotResource(path, key string, resource []byte) error {
	resources, meta, err := ReadSnapshotFile(path)
	if err != nil {
		return err
	}
	bindings, err := decodeSnapshotBindings(resources, meta)
	if err != nil {
		return fmt.Errorf("snapshot %s: %w", path, err)
	}
	metadata := make(map[string]any, len(meta))
	for k, value := range meta {
		metadata[k] = value
	}
	resources[key] = resource
	if bindings != nil {
		bindings[key] = key
		metadata[SnapshotBindingsKey] = bindings
	}
	return WriteSnapshotFile(path, resources, metadata)
}

func decodeSnapshotBindings(resources RawResources, meta map[string]json.RawMessage) (map[string]string, error) {
	raw, ok := meta[SnapshotBindingsKey]
	if !ok {
		return nil, nil
	}
	var bindings map[string]string
	if err := json.Unmarshal(raw, &bindings); err != nil || bindings == nil {
		return nil, fmt.Errorf("%s must be an object mapping component keys to resource keys", SnapshotBindingsKey)
	}
	for name, key := range bindings {
		if name == "" || key == "" {
			return nil, fmt.Errorf("%s contains an empty component or resource key", SnapshotBindingsKey)
		}
		if _, ok := resources[key]; !ok {
			return nil, fmt.Errorf("binding for %q references missing resource %q", name, key)
		}
	}
	return bindings, nil
}

// snapshotFields is shared by binding capture and static attachment. Like
// environments.CreateEnv, it includes visible fields in value-embedded structs.
func snapshotFields(env any) (reflect.Value, []reflect.StructField, error) {
	value := reflect.ValueOf(env)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, nil, fmt.Errorf("snapshot environment must be a non-nil pointer to a struct, got %T", env)
	}
	value = value.Elem()
	importableType := reflect.TypeOf((*outputs.Importable)(nil)).Elem()
	var fields []reflect.StructField
	for _, field := range reflect.VisibleFields(value.Type()) {
		if field.IsExported() && field.Type.Kind() == reflect.Pointer && field.Type.Elem().Kind() == reflect.Struct && field.Type.Implements(importableType) {
			fields = append(fields, field)
		}
	}
	return value, fields, nil
}

func snapshotFieldKey(field reflect.StructField) string {
	if key := field.Tag.Get("import"); key != "" {
		return key
	}
	return strings.ToLower(field.Name[:1]) + field.Name[1:]
}
