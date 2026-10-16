// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package schemaimpl implements the schema component, validating configurations against the schemas embedded in the
// Agent.
package schemaimpl

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/qri-io/jsonpointer"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"

	schema "github.com/DataDog/datadog-agent/comp/core/schema/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigschema "github.com/DataDog/datadog-agent/pkg/config/schema"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// Provides defines the output of the schema component
type Provides struct {
	Comp schema.Component
}

// schemaGetter returns the compiled schema for a schema ID. Schemas are compiled on demand and not kept: a compiled
// core schema adds several MiB to the heap. It's a variable so tests can use their own schema.
var schemaGetter = loadSchema

type schemaValidator struct {
	m sync.Mutex
	// violations holds the result of the last validation of each schema
	violations map[model.SchemaID][]*model.Violation
}

// NewComponent returns the schema component
func NewComponent() Provides {
	return Provides{Comp: &schemaValidator{violations: map[model.SchemaID][]*model.Violation{}}}
}

// loadSchema decompresses, parses and compiles an embedded schema
func loadSchema(schemaID model.SchemaID) (*jsonschema.Schema, error) {
	rawSchema, err := pkgconfigschema.GetRawSchema(schemaID)
	if err != nil {
		return nil, err
	}

	var loadedSchema map[string]interface{}
	err = yaml.Unmarshal(rawSchema, &loadedSchema)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal schema '%s': %s", schemaID, err)
	}

	name := string(schemaID)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, loadedSchema); err != nil {
		return nil, fmt.Errorf("could not add schema resource '%s': %s", schemaID, err)
	}
	res, err := c.Compile(name)
	if err != nil {
		return nil, fmt.Errorf("could not compile schema '%s': %s", schemaID, err)
	}

	return res, nil
}

// ValidateYAML validates raw YAML/JSON contents against a specific schema
func (s *schemaValidator) ValidateYAML(data []byte, schemaID model.SchemaID) ([]*model.Violation, error) {
	var config interface{}
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML for schema '%s': %s", schemaID, err)
	}

	sch, err := schemaGetter(schemaID)
	if err != nil {
		return nil, err
	}
	return validateData(sch, nil, config, false)
}

// ValidateConfig validates the settings of config against its schema, records the violations and returns them. It
// expects to be called once the configuration file and environment variables are loaded, before secrets and runtime
// value are resolved.
func (s *schemaValidator) ValidateConfig(config model.Reader) ([]*model.Violation, error) {
	schemaID := config.GetSchemaID()
	violations, err := s.validateConfig(config, schemaID)

	s.m.Lock()
	defer s.m.Unlock()
	if err != nil {
		// don't keep reporting the violations of a previous validation
		delete(s.violations, schemaID)
		return nil, err
	}

	s.violations[schemaID] = violations
	return violations, nil
}

// GetViolations returns the violations recorded by the last validation of a configuration following schemaID
func (s *schemaValidator) GetViolations(schemaID model.SchemaID) []*model.Violation {
	s.m.Lock()
	defer s.m.Unlock()
	// callers get their own slice so they can't change the recorded one
	return slices.Clone(s.violations[schemaID])
}

func (s *schemaValidator) validateConfig(config model.Reader, schemaID model.SchemaID) ([]*model.Violation, error) {
	cfg := config.AllSettingsWithoutDefault()
	if len(cfg) == 0 {
		return nil, nil
	}

	normalized, err := normalizeForSchema(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalidconfig: normalize config: %w", err)
	}

	sch, err := schemaGetter(schemaID)
	if err != nil {
		return nil, err
	}
	return validateData(sch, config, normalized, true)
}

// normalizeForSchema coerces a Go-native config map into JSON-native types.
func normalizeForSchema(in map[string]any) (map[string]any, error) {
	b, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := yaml.Unmarshal(b, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// collectViolations appends a violation for every leaf of ve to out. When skipSecretPlaceholders is true, the leaves
// whose value in data is an ENC[] secret placeholder are skipped.
func collectViolations(ve *jsonschema.ValidationError, cfg model.Reader, data interface{}, skipSecretPlaceholders bool, out *[]*model.Violation) {
	if len(ve.Causes) == 0 {
		if skipSecretPlaceholders && isSecretPlaceholder(data, ve.InstanceLocation) {
			return
		}
		settingKey := ""
		if cfg != nil {
			key := ""
			for _, chunk := range jsonpointer.Pointer(ve.InstanceLocation) {
				if key != "" {
					key += "."
				}
				key += chunk
				if cfg.IsSetting(key) {
					settingKey = key
					break
				}
			}
		}
		path := jsonpointer.Pointer(ve.InstanceLocation).String()
		violation := &model.Violation{
			Message:    scrubMessage(ve.Error(), path),
			Path:       path,
			SettingKey: settingKey,
		}

		if typeError, ok := ve.ErrorKind.(*kind.Type); ok {
			violation.ActualType = typeError.Got
			violation.ExpectedTypes = typeError.Want
		}
		*out = append(*out, violation)
		return
	}
	for _, cause := range ve.Causes {
		collectViolations(cause, cfg, data, skipSecretPlaceholders, out)
	}
}

// validateData validates data against sch. When skipSecretPlaceholders is true, violations on ENC[] secret
// placeholders aren't reported.
func validateData(sch *jsonschema.Schema, cfg model.Reader, data interface{}, skipSecretPlaceholders bool) ([]*model.Violation, error) {
	if sch == nil {
		return nil, errors.New("no embedded schema")
	}

	err := sch.Validate(data)
	if err == nil {
		return nil, nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return nil, err
	}
	var out []*model.Violation
	collectViolations(ve, cfg, data, skipSecretPlaceholders, &out)
	return out, nil
}

// scrubMessage scrubs the credentials from a validator message, which can contain the configured value
func scrubMessage(message string, path string) string {
	scrubbed, err := scrubber.ScrubString(message)
	if err != nil {
		return fmt.Sprintf("at '%s': invalid value", path)
	}
	return scrubbed
}

// isSecretPlaceholder returns true when the value at location in data is an ENC[] secret placeholder. The
// configuration is validated before secrets are resolved, so a placeholder is not the actual value of the setting.
func isSecretPlaceholder(data interface{}, location []string) bool {
	value, err := jsonpointer.Pointer(location).Eval(data)
	text, ok := value.(string)
	return err == nil && ok && scrubber.IsEnc(text)
}
