// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package environments contains the definitions of the different environments that can be used in a test.
package environments

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

const (
	importKey = "import"
)

func CreateEnv[Env any]() (*Env, []reflect.StructField, []reflect.Value, error) {
	var env Env

	envFields := reflect.VisibleFields(reflect.TypeOf(&env).Elem())
	envValue := reflect.ValueOf(&env)

	retainedFields := make([]reflect.StructField, 0)
	retainedValues := make([]reflect.Value, 0)
	for _, field := range envFields {
		if !field.IsExported() {
			continue
		}

		importKeyFromTag := field.Tag.Get(importKey)
		isImportable := field.Type.Implements(reflect.TypeOf((*components.Importable)(nil)).Elem())
		isPtrImportable := reflect.PointerTo(field.Type).Implements(reflect.TypeOf((*components.Importable)(nil)).Elem())

		// Produce meaningful error in case we have an importKey but field is not importable
		if importKeyFromTag != "" && !isImportable {
			return nil, nil, nil, fmt.Errorf("resource named %s has %s key but does not implement Importable interface", field.Name, importKey)
		}

		if !isImportable && isPtrImportable {
			return nil, nil, nil, fmt.Errorf("resource named %s of type %T implements Importable on pointer receiver but is not a pointer", field.Name, field.Type)
		}

		if !isImportable {
			continue
		}

		// Create zero-value if not created (pointer to struct)
		fieldValue := envValue.Elem().FieldByIndex(field.Index)
		if fieldValue.IsNil() {
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}

		retainedFields = append(retainedFields, field)
		retainedValues = append(retainedValues, fieldValue)
	}

	return &env, retainedFields, retainedValues, nil
}

// ImportKeys snapshots the field→import-key mapping from an already-provisioned
// environment. It reflects over env's exported fields using the same importable
// detection as [CreateEnv]: for each non-nil field that implements
// [components.Importable] and whose [components.Importable.Key] is non-empty,
// it records the mapping {FieldName: key}. Fields that are nil or have an empty
// key are silently skipped.
//
// The returned map can be persisted and later passed to
// [standalone.HydrateFromResources] so that import keys are replayed without
// needing a Pulumi program run.
func ImportKeys(env any) map[string]string {
	keys := make(map[string]string)
	if env == nil {
		return keys
	}

	v := reflect.ValueOf(env)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return keys
		}
		v = v.Elem()
	}

	t := v.Type()
	for _, field := range reflect.VisibleFields(t) {
		if !field.IsExported() {
			continue
		}
		fv := v.FieldByIndex(field.Index)
		if !field.Type.Implements(reflect.TypeOf((*components.Importable)(nil)).Elem()) {
			continue
		}
		if fv.IsNil() {
			continue
		}
		imp := fv.Interface().(components.Importable)
		if k := imp.Key(); k != "" {
			keys[field.Name] = k
		}
	}
	return keys
}

// BuildEnvFromResources imports the raw resources returned by a provisioner into the
// importable fields of an environment, and initializes each imported component.
//
// fields and values are the importable fields/values returned by [CreateEnv]; resources
// is the [provisioners.RawResources] map returned by the provisioner. ctx is passed to
// every component implementing [common.Initializable]. It is decoupled from *testing.T so
// non-test callers (e.g. a standalone binary) can drive provisioning.
func BuildEnvFromResources(ctx common.Context, resources provisioners.RawResources, fields []reflect.StructField, values []reflect.Value) error {
	if len(fields) != len(values) {
		panic("fields and values must have the same length")
	}

	if len(resources) == 0 {
		return nil
	}

	for idx, fieldValue := range values {
		field := fields[idx]
		importKeyFromTag := field.Tag.Get(importKey)

		// If a field value is nil, it means that it was explicitly set to nil by provisioners, hence not available
		// We should not find it in the resources map, returning an error in this case.
		if fieldValue.IsNil() {
			if _, found := resources[importKeyFromTag]; found {
				return fmt.Errorf("resource named %s has key %s but is nil", fields[idx].Name, importKeyFromTag)
			}

			continue
		}

		importable := fieldValue.Interface().(components.Importable)
		resourceKey := importable.Key()
		if importKeyFromTag != "" {
			resourceKey = importKeyFromTag
		}
		if resourceKey == "" {
			return fmt.Errorf("resource named %s has no import key set and no annotation", field.Name)
		}

		if rawResource, found := resources[resourceKey]; found {
			err := importable.Import(rawResource, fieldValue.Interface())
			if err != nil {
				return fmt.Errorf("failed to import resource named: %s with key: %s, err: %w", field.Name, resourceKey, err)
			}

			// See if the component requires init
			if initializable, ok := fieldValue.Interface().(common.Initializable); ok {
				if err := initializable.Init(ctx); err != nil {
					return fmt.Errorf("failed to init resource named: %s with key: %s, err: %w", field.Name, resourceKey, err)
				}
			}
		} else {
			return fmt.Errorf("expected resource named: %s with key: %s but not returned by provisioners", field.Name, resourceKey)
		}
	}

	return nil
}
