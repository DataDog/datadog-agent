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
	"strconv"
	"strings"

	"github.com/DataDog/zstd"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"
)

//go:embed all:compressed
var schemas embed.FS

func getSchema(name string) ([]byte, error) {
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
	uncompressSchema, err := getSchema(name)
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

// Violation is a leaf validation error with fields that callers can safely
// consume without parsing the human-readable Message.
type Violation struct {
	Message       string
	Path          string
	Rule          string
	ActualType    string
	ExpectedTypes []string
	Required      bool
}

func collectValidationErrors(ve *jsonschema.ValidationError, out *[]string) {
	if len(ve.Causes) == 0 {
		*out = append(*out, ve.Error())
		return
	}
	for _, cause := range ve.Causes {
		collectValidationErrors(cause, out)
	}
}

func collectViolations(sch *jsonschema.Schema, ve *jsonschema.ValidationError, out *[]Violation) {
	if len(ve.Causes) == 0 {
		violation := Violation{
			Message:  ve.Error(),
			Path:     jsonPointer(ve.InstanceLocation),
			Rule:     strings.Join(ve.ErrorKind.KeywordPath(), "/"),
			Required: isRequired(sch, ve.InstanceLocation),
		}
		if typeError, ok := ve.ErrorKind.(*kind.Type); ok {
			violation.ActualType = typeError.Got
			violation.ExpectedTypes = append([]string(nil), typeError.Want...)
		}
		*out = append(*out, violation)
		return
	}
	for _, cause := range ve.Causes {
		collectViolations(sch, cause, out)
	}
}

func jsonPointer(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, token := range tokens {
		builder.WriteByte('/')
		builder.WriteString(strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1"))
	}
	return builder.String()
}

func isRequired(sch *jsonschema.Schema, path []string) bool {
	if len(path) == 0 {
		return false
	}

	current := dereferenceSchema(sch)
	for index, token := range path {
		if current == nil {
			return false
		}
		if index == len(path)-1 {
			return contains(current.Required, token)
		}
		current = dereferenceSchema(schemaAt(current, token))
	}
	return false
}

func dereferenceSchema(sch *jsonschema.Schema) *jsonschema.Schema {
	seen := make(map[*jsonschema.Schema]struct{})
	for sch != nil && sch.Ref != nil {
		if _, found := seen[sch]; found {
			return sch
		}
		seen[sch] = struct{}{}
		sch = sch.Ref
	}
	return sch
}

func schemaAt(sch *jsonschema.Schema, token string) *jsonschema.Schema {
	if property, found := sch.Properties[token]; found {
		return property
	}

	index, err := strconv.Atoi(token)
	if err != nil || index < 0 {
		return nil
	}
	if items, ok := sch.Items.([]*jsonschema.Schema); ok && index < len(items) {
		return items[index]
	}
	if items, ok := sch.Items.(*jsonschema.Schema); ok {
		return items
	}
	if index < len(sch.PrefixItems) {
		return sch.PrefixItems[index]
	}
	return sch.Items2020
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validateData(sch *jsonschema.Schema, config interface{}) ([]string, error) {
	if sch == nil {
		return nil, errors.New("no embedded schema")
	}

	err := sch.Validate(config)
	if err == nil {
		return nil, err
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []string{err.Error()}, nil
	}
	var out []string
	collectValidationErrors(ve, &out)
	return out, nil
}

func validateDataDetailed(sch *jsonschema.Schema, config interface{}) ([]Violation, error) {
	if sch == nil {
		return nil, errors.New("no embedded schema")
	}

	err := sch.Validate(config)
	if err == nil {
		return nil, nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []Violation{{Message: err.Error()}}, nil
	}
	var out []Violation
	collectViolations(sch, ve, &out)
	return out, nil
}

// ValidateCoreConfig validates a unmarshal YAML/JSON contents against the core agent schema
func ValidateCoreConfig(config interface{}) ([]string, error) {
	violations, err := ValidateCoreConfigDetailed(config)
	if err != nil {
		return nil, err
	}
	errors := make([]string, len(violations))
	for index, violation := range violations {
		errors[index] = violation.Message
	}
	return errors, nil
}

// ValidateCoreConfigDetailed validates unmarshaled YAML/JSON contents against
// the core Agent schema and returns the individual validation leaves.
func ValidateCoreConfigDetailed(config interface{}) ([]Violation, error) {
	sch, err := coreSchemaGetter()
	if err != nil {
		return nil, err
	}
	return validateDataDetailed(sch, config)
}

// ValidateSystemProbeConfig validates a unmarshal YAML/JSON contents against the system-probe agent schema
func ValidateSystemProbeConfig(config interface{}) ([]string, error) {
	sch, err := sysprobeSchemaGetter()
	if err != nil {
		return nil, err
	}
	return validateData(sch, config)
}

// GetCoreSchema returns the raw bytes of the embedded core agent configuration schema.
func GetCoreSchema() ([]byte, error) { return getSchema("core_schema") }

// GetSystemProbeSchema returns the raw bytes of the embedded system-probe configuration schema.
func GetSystemProbeSchema() ([]byte, error) { return getSchema("system-probe_schema") }
