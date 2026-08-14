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
	"os"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
)

const (
	staticStackProvisionerDefaultID = "static-stack"
)

// StaticStackProvisioner is a provisioner that reads a single JSON file and
// populates a typed environment directly.
//
// # JSON file format
//
// The file must be a JSON object whose top-level keys are resource names.  Each
// value is the raw JSON payload for that resource.  Keys prefixed with "_" are
// treated as metadata and are ignored.
//
//	{
//	  "_source": "pulumi-stack-my-stack",
//	  "kubernetesCluster": { "clusterName": "my-cluster", "kubeConfig": "…" },
//	  "fakeIntake":        { "host": "localhost", "port": 8080 },
//	  "agent":             { "version": "7.x" }
//	}
//
// # Field naming and key resolution
//
// For each exported pointer field in *Env that implements [components.Importable],
// the provisioner derives a resource key using the following priority order:
//
//  1. The value of the `import` struct tag, when present.
//  2. The field name with its first letter lowercased (lowerCamelCase).
//
// The derived key is then looked up in the JSON object:
//   - Match found: [components.Importable.SetKey] is called so that
//     [environments.BuildEnvFromResources] can unmarshal the payload into the field.
//   - No match: the field is set to nil and silently skipped.
//
// # Naming convention
//
// For the provisioner to wire a field automatically, the corresponding JSON key
// must equal the lowerCamelCase form of the Go field name — unless the field
// carries an explicit `import` tag that overrides it.
//
// Given this environment struct:
//
//	type MyEnv struct {
//	    KubernetesCluster *components.KubernetesCluster `import:"kubernetesCluster"` // explicit tag
//	    FakeIntake        *components.FakeIntake                                      // → key "fakeIntake"
//	    Agent             *components.KubernetesAgent                                 // → key "agent"
//	}
//
// The expected JSON keys are "kubernetesCluster", "fakeIntake", and "agent".
// A field whose JSON key is absent from the file is set to nil (not an error);
// a JSON key that has no matching field is silently ignored.
//
// Use the `import` tag when the field name and the JSON key diverge — for
// example when a legacy snapshot uses a different naming convention, or when
// two fields of the same type would otherwise produce duplicate keys.
//
// # Embedded structs and nesting
//
// The provisioner inspects only the direct fields of *Env — it does not recurse
// into embedded structs.
//
// Value-embedded structs (e.g. CoverageBase in environments.Host) are silently
// skipped because they have kind Struct, not Ptr.  This is harmless as long as
// those helpers carry no [components.Importable] fields themselves.
//
//	// OK — CoverageBase is a value embed with no Importable fields.
//	// wireEnv skips it and still finds RemoteHost, FakeIntake, Agent, Updater.
//	type Host struct {
//	    CoverageBase                      // skipped (Struct, not Ptr)
//	    RemoteHost *components.RemoteHost // → key "remoteHost"
//	    FakeIntake *components.FakeIntake // → key "fakeIntake"
//	    Agent      *components.RemoteHostAgent  // → key "agent"
//	    Updater    *components.RemoteHostUpdater // → key "updater"
//	}
//
// Embedding another environment struct by value does NOT work: its component
// pointer fields are invisible to wireEnv because Go reflection reports only
// the direct (non-promoted) fields at the outermost level.
//
//	// NOT OK — RemoteHost, FakeIntake, Agent, Updater inside Host are never seen.
//	type ExtendedHost struct {
//	    Host                            // skipped (Struct, not Ptr); its fields are invisible
//	    ExtraComp *components.FakeIntake // → key "extraComp" (only this is wired)
//	}
//
// If you need to extend an existing environment, declare all component pointer
// fields directly on the outer struct and use `import` tags where the JSON keys
// must match a specific name.
//
// This design means no `import` struct tags need to be added to built-in
// environment types such as [environments.Kubernetes], and no Pulumi provisioner
// code needs to change.
type StaticStackProvisioner[Env any] struct {
	id       string
	filePath string
}

var _ TypedProvisioner[any] = &StaticStackProvisioner[any]{}

// NewStaticStackProvisioner returns a new StaticStackProvisioner.
// Pass an empty id to use the default ("static-stack").
// filePath must be the path to a single JSON descriptor file.
func NewStaticStackProvisioner[Env any](id string, filePath string) *StaticStackProvisioner[Env] {
	if id == "" {
		id = staticStackProvisionerDefaultID
	}
	return &StaticStackProvisioner[Env]{
		id:       id,
		filePath: filePath,
	}
}

// ID returns the provisioner's identifier.
func (fp *StaticStackProvisioner[Env]) ID() string {
	return fp.id
}

// ProvisionEnv reads the JSON file, expands its top-level keys into [RawResources],
// and wires the matching fields in *env.
func (fp *StaticStackProvisioner[Env]) ProvisionEnv(_ context.Context, _ string, _ io.Writer, env *Env) (RawResources, error) {
	resources, err := fp.readResources()
	if err != nil {
		return nil, err
	}

	if err := fp.wireEnv(env, resources); err != nil {
		return nil, err
	}

	return resources, nil
}

// Destroy is a no-op for the StaticStackProvisioner.
func (fp *StaticStackProvisioner[Env]) Destroy(context.Context, string, io.Writer) error {
	return nil
}

// readResources reads the single JSON file and expands its top-level keys into
// separate RawResources entries.  Keys prefixed with "_" are ignored.
func (fp *StaticStackProvisioner[Env]) readResources() (RawResources, error) {
	fmt.Printf("Reading file: %s\n", fp.filePath)
	data, err := os.ReadFile(fp.filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", fp.filePath, err)
	}

	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(data, &topLevel); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", fp.filePath, err)
	}

	resources := make(RawResources, len(topLevel))
	for key, value := range topLevel {
		if strings.HasPrefix(key, "_") {
			continue // skip metadata fields (e.g. _source)
		}
		resources[key] = []byte(value)
	}
	return resources, nil
}

// wireEnv iterates over the exported fields of *Env.  For each field that
// implements [components.Importable] it resolves a resource key (see the
// [StaticStackProvisioner] type-level doc for the full naming rules) and then:
//   - match found: calls SetKey so that BuildEnvFromResources can locate and
//     unmarshal the payload.
//   - no match: sets the field to nil so that BuildEnvFromResources skips it
//     without error.
func (fp *StaticStackProvisioner[Env]) wireEnv(env *Env, resources RawResources) error {
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
