// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
)

const (
	typedFileProvisionerDefaultID = "typed-file"
)

// TypedFileProvisioner is a provisioner that reads JSON files from a filesystem and
// populates a typed environment directly.
//
// Each JSON file is treated as a multi-resource document: its top-level keys become
// independent entries in [RawResources] (keys prefixed with "_" are skipped as
// metadata).  This mirrors the snapshot format produced by real Pulumi stacks, where
// a single JSON file contains one object per deployed component.
//
// During [ProvisionEnv] the provisioner also walks the exported fields of *Env.  For
// every field that implements [components.Importable] it derives a canonical resource
// key (the `import` struct tag when present, otherwise the field name with its first
// letter lowercased).  If a matching resource exists the field's key is set via
// [components.Importable.SetKey]; otherwise the field is set to nil so that
// [environments.BuildEnvFromResources] skips it gracefully.
//
// This design means no `import` struct tags need to be added to built-in environment
// types such as [environments.Kubernetes], and no Pulumi provisioner code needs to
// change.
type TypedFileProvisioner[Env any] struct {
	id string
	fs fs.FS
}

var _ TypedProvisioner[any] = &TypedFileProvisioner[any]{}

// NewTypedFileProvisioner returns a new TypedFileProvisioner.
// Pass an empty id to use the default ("typed-file").
func NewTypedFileProvisioner[Env any](id string, filesystem fs.FS) *TypedFileProvisioner[Env] {
	if id == "" {
		id = typedFileProvisionerDefaultID
	}
	return &TypedFileProvisioner[Env]{
		id: id,
		fs: filesystem,
	}
}

// ID returns the provisioner's identifier.
func (fp *TypedFileProvisioner[Env]) ID() string {
	return fp.id
}

// ProvisionEnv reads all JSON files from the filesystem, expands each file's
// top-level keys into [RawResources], and wires the matching fields in *env.
func (fp *TypedFileProvisioner[Env]) ProvisionEnv(_ context.Context, _ string, _ io.Writer, env *Env) (RawResources, error) {
	resources, err := fp.readResources()
	if err != nil {
		return nil, err
	}

	if err := fp.wireEnv(env, resources); err != nil {
		return nil, err
	}

	return resources, nil
}

// Destroy is a no-op for the TypedFileProvisioner.
func (fp *TypedFileProvisioner[Env]) Destroy(context.Context, string, io.Writer) error {
	return nil
}

// readResources walks the FS and expands each JSON file's top-level keys into
// separate RawResources entries.  Keys prefixed with "_" are ignored.
func (fp *TypedFileProvisioner[Env]) readResources() (RawResources, error) {
	resources := make(RawResources)

	err := fs.WalkDir(fp.fs, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fs.SkipDir
		}
		if !d.Type().IsRegular() || filepath.Ext(path) != fileExtFilter {
			return nil
		}

		data, err := fs.ReadFile(fp.fs, path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		var topLevel map[string]json.RawMessage
		if err := json.Unmarshal(data, &topLevel); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		for key, value := range topLevel {
			if strings.HasPrefix(key, "_") {
				continue // skip metadata fields (e.g. _source)
			}
			resources[key] = []byte(value)
		}
		return nil
	})
	return resources, err
}

// wireEnv iterates over the exported fields of *Env.  For each field that
// implements [components.Importable]:
//   - if a matching resource key exists: calls SetKey so that
//     BuildEnvFromResources can locate and unmarshal it.
//   - if no matching resource key exists: sets the field to nil so that
//     BuildEnvFromResources skips it without error.
func (fp *TypedFileProvisioner[Env]) wireEnv(env *Env, resources RawResources) error {
	importableType := reflect.TypeOf((*components.Importable)(nil)).Elem()

	envValue := reflect.ValueOf(env).Elem()
	envType := envValue.Type()

	for i := range envType.NumField() {
		field := envType.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldValue := envValue.Field(i)

		// Only handle pointer-to-struct fields (all component types are pointers).
		if fieldValue.Kind() != reflect.Ptr {
			continue
		}

		if !field.Type.Implements(importableType) {
			continue
		}

		// Derive the canonical resource key: prefer the `import` tag, fall back
		// to the field name with its first letter lowercased.
		key := field.Tag.Get("import")
		if key == "" {
			name := field.Name
			key = strings.ToLower(name[:1]) + name[1:]
		}

		if _, found := resources[key]; found {
			if fieldValue.IsNil() {
				fieldValue.Set(reflect.New(field.Type.Elem()))
			}
			fieldValue.Interface().(components.Importable).SetKey(key)
		} else {
			// Mark the component as not provisioned so BuildEnvFromResources skips it.
			fieldValue.Set(reflect.Zero(field.Type))
		}
	}
	return nil
}
