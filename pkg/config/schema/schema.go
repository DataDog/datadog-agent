// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package schema embeds the Agent configuration schemas and provides
// functions to retrieve them and to validate configuration data against them.
package schema

import (
	"embed"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/zstd"

	"github.com/qri-io/jsonpointer"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"
)

//go:embed all:compressed
var schemas embed.FS

func getRawSchema(name string) ([]byte, error) {
	data, err := schemas.ReadFile("compressed/" + name + ".yaml.zstd")
	if err != nil {
		return nil, fmt.Errorf("no embedded schema for %s", name)
	}

	uncompressSchema, err := zstd.Decompress(nil, data)
	if err != nil {
		return nil, fmt.Errorf("could not Decompress schema '%s': %s", name, err)
	}

	return uncompressSchema, nil
}

func loadSchema(name string) (*jsonschema.Schema, error) {
	uncompressSchema, err := getRawSchema(name)
	if err != nil {
		return nil, err
	}

	var loadSchema map[string]interface{}
	err = yaml.Unmarshal(uncompressSchema, &loadSchema)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal schema '%s': %s", name, err)
	}

	c := jsonschema.NewCompiler()

	if err := c.AddResource(name, loadSchema); err != nil {
		return nil, fmt.Errorf("could not add schema resource '%s': %s", name, err)
	}
	res, err := c.Compile(name)
	if err != nil {
		return nil, fmt.Errorf("could not compile schema '%s': %s", name, err)
	}

	return res, nil
}

func getCoreSchema() (*jsonschema.Schema, error) {
	return loadSchema("core_schema")
}

func getSysprobeSchema() (*jsonschema.Schema, error) {
	return loadSchema("system-probe_schema")
}

var (
	coreSchemaGetter     = getCoreSchema
	sysprobeSchemaGetter = getSysprobeSchema
)

func getSchema(schemaID model.SchemaID) (*jsonschema.Schema, error) {
	if schemaID == model.CoreSchema {
		return coreSchemaGetter()
	} else if schemaID == model.SystemProbeSchema {
		return sysprobeSchemaGetter()
	}
	return nil, fmt.Errorf("unknown schema ID '%s'", schemaID)
}

// Violation is a validation error with structured data and a human-readable Message
// consume without parsing the human-readable Message.
type Violation struct {
	Message       string
	Path          string
	ActualType    string
	ExpectedTypes []string
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

func collectViolations(ve *jsonschema.ValidationError, out *[]Violation) {
	if len(ve.Causes) == 0 {
		violation := Violation{
			Message: ve.Error(),
			Path:    jsonpointer.Pointer(ve.InstanceLocation).String(),
		}
		if typeError, ok := ve.ErrorKind.(*kind.Type); ok {
			violation.ActualType = typeError.Got
			violation.ExpectedTypes = typeError.Want
		}
		*out = append(*out, violation)
		return
	}
	for _, cause := range ve.Causes {
		collectViolations(cause, out)
	}
}

func validateData(sch *jsonschema.Schema, data interface{}) ([]Violation, error) {
	if sch == nil {
		return nil, errors.New("no embedded schema")
	}

	err := sch.Validate(data)
	if err == nil {
		return nil, nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []Violation{{Message: err.Error()}}, nil
	}
	var out []Violation
	collectViolations(ve, &out)
	return out, nil
}

// ValidateYAML validates raw YAML/JSON contents against a specific schema
func ValidateYAML(data []byte, schemaID model.SchemaID) ([]Violation, error) {
	var config interface{}
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML for schema '%s': %s", schemaID, err)
	}

	sch, err := getSchema(schemaID)
	if err != nil {
		return nil, err
	}
	return validateData(sch, config)
}

// customerConfig returns the settings set by the customer (file, env vars, secrets, ...etc), excluding defaults.
// Settings rewritten at the agent-runtime layer by post-load processing (ie: Adjust()) are reverted to the value from
// the highest customer layer, so the schema sees the customer's actual configuration and not values repaired by the
// agent.
func customerConfig(config model.Reader) map[string]any {
	settings := config.AllSettingsWithoutDefault()
	bySource := config.AllSettingsBySource()
	runtime, _ := bySource[model.SourceAgentRuntime].(map[string]any)
	for _, path := range settingPaths(config, runtime, nil) {
		if config.GetSource(strings.Join(path, ".")) != model.SourceAgentRuntime {
			continue
		}
		if value, ok := customerValue(bySource, path); ok {
			setPath(settings, path, value)
		} else {
			deletePath(settings, path)
		}
	}
	return settings
}

// settingPaths returns the path of every setting found in tree
func settingPaths(config model.Reader, tree map[string]any, prefix []string) [][]string {
	var paths [][]string
	for key, value := range tree {
		path := append(slices.Clone(prefix), key)
		if child, ok := value.(map[string]any); ok && !config.IsSetting(strings.Join(path, ".")) {
			paths = append(paths, settingPaths(config, child, path)...)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

// customerValue returns the value at path from the highest priority customer layer, ie: below the agent-runtime layer
// and excluding defaults.
func customerValue(bySource map[model.Source]any, path []string) (any, bool) {
	end := slices.Index(model.Sources, model.SourceAgentRuntime)
	for i := end - 1; i >= 0; i-- {
		if model.Sources[i] == model.SourceDefault {
			continue
		}
		node, ok := bySource[model.Sources[i]]
		for _, key := range path {
			if !ok {
				break
			}
			var tree map[string]any
			if tree, ok = node.(map[string]any); ok {
				node, ok = tree[key]
			}
		}
		if ok {
			return node, true
		}
	}
	return nil, false
}

func setPath(tree map[string]any, path []string, value any) {
	for _, key := range path[:len(path)-1] {
		child, ok := tree[key].(map[string]any)
		if !ok {
			child = map[string]any{}
			tree[key] = child
		}
		tree = child
	}
	tree[path[len(path)-1]] = value
}

func deletePath(tree map[string]any, path []string) {
	for _, key := range path[:len(path)-1] {
		child, ok := tree[key].(map[string]any)
		if !ok {
			return
		}
		tree = child
	}
	delete(tree, path[len(path)-1])
}

// ValidateConfig validates the settings set by the customer against a specific schema and returns the individual
// validation leaves.
func ValidateConfig(config model.Reader, ID model.SchemaID) ([]Violation, error) {
	cfg := customerConfig(config)
	if len(cfg) == 0 {
		return nil, nil
	}

	normalized, err := normalizeForSchema(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalidconfig: normalize config: %w", err)
	}

	sch, err := getSchema(ID)
	if err != nil {
		return nil, err
	}
	res, err := validateData(sch, normalized)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(res, func(violation Violation) bool {
		return isUnresolvedSecret(config, normalized, violation)
	}), nil
}

// Skip type errors for placeholders, but validate ENC-looking values returned by the secret backend.
func isUnresolvedSecret(config model.Reader, normalized map[string]any, violation Violation) bool {
	if violation.ActualType != "string" {
		return false
	}
	pointer, err := jsonpointer.Parse(violation.Path)
	if err != nil {
		return false
	}
	value, err := pointer.Eval(normalized)
	text, ok := value.(string)
	if err != nil || !ok || !scrubber.IsEnc(text) {
		return false
	}
	// A compound setting carries the source on its enclosing map or list.
	for i := len(pointer); i > 0; i-- {
		key := strings.Join(pointer[:i], ".")
		if config.IsSetting(key) {
			return config.GetSource(key) != model.SourceSecret
		}
	}
	return false
}

// GetRawSchema returns the raw bytes of an embedded schema.
func GetRawSchema(ID model.SchemaID) ([]byte, error) {
	if ID == model.CoreSchema {
		return getRawSchema("core_schema")
	} else if ID == model.SystemProbeSchema {
		return getRawSchema("system-probe_schema")
	}
	return nil, fmt.Errorf("unknown schema ID '%s'", ID)
}
