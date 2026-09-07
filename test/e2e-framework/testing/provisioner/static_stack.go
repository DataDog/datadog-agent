// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioner

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"slices"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
)

const staticStackProvisionerDefaultID = "static-stack"

// StaticStackProvisioner reads a snapshot and binds its resources to a typed
// environment without running Pulumi. Destroy is a no-op: the caller owns the
// existing infrastructure.
//
// # Snapshot format
//
// Top-level keys name resources; keys starting with "_" are metadata. The
// optional _bindings object maps component keys to exported resource names:
//
//	{
//	  "_bindings": {"remoteHost": "dd-Host-aws-vm"},
//	  "dd-Host-aws-vm": {"address": "example", "port": 22, "username": "ubuntu"}
//	}
//
// WriteSnapshotFileForEnv captures bindings from a provisioned environment.
// Without bindings, existing canonical snapshots are still supported: a field
// named RemoteHost looks up "remoteHost", or the field's explicit `import` tag.
// Unknown resource names are never guessed by inspecting their contents.
//
// # Component fields
//
// Exported pointer fields implementing outputs.Importable are bound, including
// visible fields in value-embedded structs. Missing optional components are set
// to nil. A binding to a missing resource is an error; a non-empty snapshot with
// no matching component fields is also an error rather than a silently empty
// environment. Callers must still check the components required by their
// operation (for example RemoteHost for a host installation).
//
// An explicit import tag takes precedence, matching BuildEnvFromResources. Its
// binding, if present, must refer to the tagged key itself. A nil embedded parent
// returns an error instead of panicking during reflection.
type StaticStackProvisioner[Env any] struct {
	id       string
	filePath string
}

var _ TypedProvisioner[any] = &StaticStackProvisioner[any]{}

// NewStaticStackProvisioner returns a provisioner for a single snapshot file.
// Pass an empty id to use the default ("static-stack").
func NewStaticStackProvisioner[Env any](id string, filePath string) *StaticStackProvisioner[Env] {
	if id == "" {
		id = staticStackProvisionerDefaultID
	}
	return &StaticStackProvisioner[Env]{id: id, filePath: filePath}
}

// ID returns the provisioner's identifier.
func (fp *StaticStackProvisioner[Env]) ID() string { return fp.id }

// ProvisionEnv reads the snapshot and wires its matching component fields.
func (fp *StaticStackProvisioner[Env]) ProvisionEnv(_ context.Context, _ string, logger io.Writer, env *Env) (RawResources, error) {
	if logger != nil {
		fmt.Fprintf(logger, "Reading snapshot: %s\n", fp.filePath)
	}
	resources, meta, err := ReadSnapshotFile(fp.filePath)
	if err != nil {
		return nil, err
	}
	bindings, err := decodeSnapshotBindings(resources, meta)
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", fp.filePath, err)
	}
	if err := fp.wireEnv(env, resources, bindings); err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", fp.filePath, err)
	}
	return resources, nil
}

// Destroy does not delete infrastructure when attaching to an existing snapshot.
func (fp *StaticStackProvisioner[Env]) Destroy(context.Context, string, io.Writer) error {
	return nil
}

func (fp *StaticStackProvisioner[Env]) wireEnv(env *Env, resources RawResources, bindings map[string]string) error {
	value, fields, err := snapshotFields(env)
	if err != nil {
		return err
	}
	matched := 0
	for _, field := range fields {
		component, err := value.FieldByIndexErr(field.Index)
		if err != nil {
			return fmt.Errorf("accessing component %s: %w", field.Name, err)
		}
		canonical := snapshotFieldKey(field)
		key := canonical
		if bound, ok := bindings[canonical]; ok {
			if tag := field.Tag.Get("import"); tag != "" && bound != tag {
				return fmt.Errorf("binding for %s conflicts with its import tag %q", field.Name, tag)
			}
			key = bound
		}
		if _, ok := resources[key]; ok {
			if component.IsNil() {
				component.Set(reflect.New(field.Type.Elem()))
			}
			component.Interface().(outputs.Importable).SetKey(key)
			matched++
		} else {
			component.Set(reflect.Zero(field.Type))
		}
	}
	if len(resources) > 0 && len(fields) > 0 && matched == 0 {
		keys := make([]string, 0, len(resources))
		for key := range resources {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		return fmt.Errorf("no resources match %T (available keys: %v); re-export with WriteSnapshotFileForEnv or add explicit %s mappings for this legacy snapshot", env, keys, SnapshotBindingsKey)
	}
	return nil
}
