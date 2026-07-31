// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"k8s.io/apimachinery/pkg/runtime"
)

const redisDBIntegrationName = "redisdb"

//go:embed schemas/redisdb-instance.schema.json
var redisDBInstanceSchemaDocument []byte

type integrationInstanceValidationError struct {
	Field   string
	Message string
}

type integrationInstanceValidator interface {
	Validate(integration string, instance runtime.RawExtension) []integrationInstanceValidationError
}

type jsonSchemaIntegrationInstanceValidator struct {
	redisDBInstanceSchema *jsonschema.Schema
}

var (
	integrationInstanceValidatorOnce sync.Once
	defaultInstanceValidator         integrationInstanceValidator
	defaultInstanceValidatorErr      error
)

func defaultIntegrationInstanceValidator() (integrationInstanceValidator, error) {
	integrationInstanceValidatorOnce.Do(func() {
		defaultInstanceValidator, defaultInstanceValidatorErr = newIntegrationInstanceValidator()
	})
	return defaultInstanceValidator, defaultInstanceValidatorErr
}

func newIntegrationInstanceValidator() (integrationInstanceValidator, error) {
	var schemaDocument any
	if err := json.Unmarshal(redisDBInstanceSchemaDocument, &schemaDocument); err != nil {
		return nil, fmt.Errorf("could not decode embedded Redis instance schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	const schemaURL = "embedded://redisdb-instance.schema.json"
	if err := compiler.AddResource(schemaURL, schemaDocument); err != nil {
		return nil, fmt.Errorf("could not add embedded Redis instance schema: %w", err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return nil, fmt.Errorf("could not compile embedded Redis instance schema: %w", err)
	}

	return &jsonSchemaIntegrationInstanceValidator{redisDBInstanceSchema: schema}, nil
}

func (v *jsonSchemaIntegrationInstanceValidator) Validate(integration string, instance runtime.RawExtension) []integrationInstanceValidationError {
	if strings.TrimSpace(integration) != redisDBIntegrationName {
		return nil
	}

	data, err := rawExtensionToData(instance)
	if err != nil {
		return []integrationInstanceValidationError{{Message: fmt.Sprintf("could not read Redis instance: %v", err)}}
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return []integrationInstanceValidationError{{Message: fmt.Sprintf("could not decode Redis instance: %v", err)}}
	}

	if object, ok := value.(map[string]any); ok && !hasRedisConnectionTarget(object) {
		return []integrationInstanceValidationError{{Message: "Redis instance must specify host and port, or unix_socket_path"}}
	}

	if err := v.redisDBInstanceSchema.Validate(value); err != nil {
		var validationError *jsonschema.ValidationError
		if !errors.As(err, &validationError) {
			return []integrationInstanceValidationError{{Message: fmt.Sprintf("Redis instance is invalid: %v", err)}}
		}

		var validationErrors []integrationInstanceValidationError
		collectIntegrationInstanceValidationErrors(validationError, &validationErrors)
		return validationErrors
	}

	return nil
}

func hasRedisConnectionTarget(instance map[string]any) bool {
	_, hasHost := instance["host"]
	_, hasPort := instance["port"]
	unixSocketPath, hasUnixSocketPath := instance["unix_socket_path"]
	return hasHost && hasPort || hasUnixSocketPath && unixSocketPath != nil
}

func collectIntegrationInstanceValidationErrors(err *jsonschema.ValidationError, out *[]integrationInstanceValidationError) {
	if additionalProperties, ok := err.ErrorKind.(*kind.AdditionalProperties); ok {
		for _, property := range additionalProperties.Properties {
			location := append([]string(nil), err.InstanceLocation...)
			*out = append(*out, integrationInstanceValidationError{
				Field:   jsonInstanceFieldPath(append(location, property)),
				Message: fmt.Sprintf("unknown Redis instance field %q", property),
			})
		}
		return
	}

	if len(err.Causes) > 0 {
		for _, cause := range err.Causes {
			collectIntegrationInstanceValidationErrors(cause, out)
		}
		return
	}

	*out = append(*out, integrationInstanceValidationError{
		Field:   jsonInstanceFieldPath(err.InstanceLocation),
		Message: validationErrorMessage(err),
	})
}

func validationErrorMessage(err *jsonschema.ValidationError) string {
	message := err.Error()
	if separator := strings.Index(message, ": "); separator >= 0 {
		return "invalid Redis instance value: " + message[separator+2:]
	}
	return "invalid Redis instance value: " + message
}

func jsonInstanceFieldPath(tokens []string) string {
	var path strings.Builder
	for _, token := range tokens {
		if _, err := strconv.Atoi(token); err == nil {
			fmt.Fprintf(&path, "[%s]", token)
			continue
		}
		if path.Len() > 0 {
			path.WriteByte('.')
		}
		path.WriteString(token)
	}
	return path.String()
}
