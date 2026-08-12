// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

const (
	fileProvisionerDefaultID = "file"
	fileExtFilter            = ".json"
	// importKeyTag matches environments.importKey — the same struct tag
	// BuildEnvFromResources consults when resolving a field's import key.
	importKeyTag = "import"
)

// FileProvisioner is a provisioner that reads JSON files from a filesystem.
type FileProvisioner struct {
	id string
	fs fs.FS
}

var _ Provisioner = &FileProvisioner{}

// NewFileProvisioner returns a new FileProvisioner.
func NewFileProvisioner(id string, fs fs.FS) *FileProvisioner {
	if id == "" {
		id = fileProvisionerDefaultID
	}

	return &FileProvisioner{
		id: id,
		fs: fs,
	}
}

// ID returns the ID of the provisioner.
func (fp *FileProvisioner) ID() string {
	return fp.id
}

// Provision reads JSON files from the filesystem and returns them as raw resources.
func (fp *FileProvisioner) Provision(context.Context, string, io.Writer) (RawResources, error) {
	resources := make(RawResources)

	return resources, fs.WalkDir(fp.fs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}

		if !d.Type().IsRegular() {
			return nil
		}

		if filepath.Ext(path) != fileExtFilter {
			return nil
		}

		data, err := fs.ReadFile(fp.fs, path)
		if err != nil {
			return err
		}

		// We may need to put the relative path instead of just filename
		resources[strings.TrimSuffix(d.Name(), fileExtFilter)] = data
		return nil
	})
}

// Destroy is a no-op for the FileProvisioner.
func (fp *FileProvisioner) Destroy(context.Context, string, io.Writer) error {
	return nil
}

// SingleFileProvisioner is a provisioner that reads a single JSON file whose
// top-level keys are RawResources keys (e.g. {"kubernetesCluster": {...},
// "agent": {...}}). It consumes the combined environment description
// written by an out-of-band provisioning step (see cmd/envctl), decoupling
// "how the infra was created" from "how a test reads it back".
//
// It implements TypedProvisioner[Env] rather than the simpler
// UntypedProvisioner: environments.BuildEnvFromResources looks up each
// importable field by its Importable.Key(), which for a fresh component is
// only ever set by Pulumi's components.Export() during a live pulumi.Context
// run. An UntypedProvisioner is never handed the target Env, so it has no
// way to set that key — every field would fail with "no import key set and
// no annotation". SingleFileProvisioner works around this by receiving the
// live *Env and calling SetKey itself, using the same tag-or-field-name
// convention environments.CreateEnv/BuildEnvFromResources use internally.
type SingleFileProvisioner[Env any] struct {
	id   string
	path string
	// fingerprint is the sha256 of the file's content at construction
	// time. BaseSuite.UpdateEnv/reconcileEnv decides whether to
	// re-provision purely via reflect.DeepEqual on this struct, so
	// without a content-derived field, calling UpdateEnv again with the
	// same id+path after the file changed out-of-band would always look
	// like "no change" and silently no-op.
	fingerprint string
}

var _ TypedProvisioner[struct{}] = &SingleFileProvisioner[struct{}]{}

// NewSingleFileProvisioner returns a new SingleFileProvisioner reading path.
func NewSingleFileProvisioner[Env any](id, path string) *SingleFileProvisioner[Env] {
	if id == "" {
		id = fileProvisionerDefaultID
	}
	fp := &SingleFileProvisioner[Env]{id: id, path: path}
	if data, err := os.ReadFile(path); err == nil {
		sum := sha256.Sum256(data)
		fp.fingerprint = hex.EncodeToString(sum[:])
	}
	return fp
}

// ID returns the ID of the provisioner.
func (fp *SingleFileProvisioner[Env]) ID() string { return fp.id }

// ProvisionEnv reads the JSON file, assigns each importable field of env an
// import key matching one of the file's top-level entries, and returns
// those entries as raw resources.
func (fp *SingleFileProvisioner[Env]) ProvisionEnv(_ context.Context, _ string, _ io.Writer, env *Env) (RawResources, error) {
	data, err := os.ReadFile(fp.path)
	if err != nil {
		return nil, err
	}

	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", fp.path, err)
	}

	if err := assignImportKeys(env, entries); err != nil {
		return nil, err
	}

	if fb, ok := any(env).(common.FileBacked); ok {
		fb.SetEnvFilePath(fp.path)
	}

	resources := make(RawResources, len(entries))
	for key, raw := range entries {
		resources[key] = []byte(raw)
	}
	return resources, nil
}

// Destroy is a no-op: tearing down the described infrastructure is a
// separate, explicit step — this provisioner only ever reads a description.
func (fp *SingleFileProvisioner[Env]) Destroy(context.Context, string, io.Writer) error {
	return nil
}

// assignImportKeys mirrors environments.BuildEnvFromResources's own key
// resolution (import:"..." tag, else the field name) so that each
// importable field of env gets a key matching one of entries' top-level
// keys, letting BuildEnvFromResources find it later.
func assignImportKeys(env any, entries map[string]json.RawMessage) error {
	envValue := reflect.ValueOf(env).Elem()
	envType := envValue.Type()
	importableType := reflect.TypeFor[components.Importable]()

	for _, field := range reflect.VisibleFields(envType) {
		if !field.IsExported() || !field.Type.Implements(importableType) {
			continue
		}

		fieldValue := envValue.FieldByIndex(field.Index)
		if fieldValue.IsNil() {
			continue
		}

		key := field.Tag.Get(importKeyTag)
		if key == "" {
			key = lowerCamel(field.Name)
		}
		if _, ok := entries[key]; !ok {
			continue
		}

		fieldValue.Interface().(components.Importable).SetKey(key)
	}
	return nil
}

func lowerCamel(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
