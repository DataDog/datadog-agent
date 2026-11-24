// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schemaimpl

import (
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// TestDefaultConfigMatchesSchema validates the default configuration of the Agent, as built on the current platform,
// against the schema embedded in the Agent. At runtime the Agent only validates the settings a user sets, so a default
// breaking the schema would go unnoticed until a tool validating a full configuration (IDE, CI, ...) rejects it.
func TestDefaultConfigMatchesSchema(t *testing.T) {
	for _, tc := range []struct {
		name     string
		schemaID model.SchemaID
		newCfg   func(testing.TB) model.BuildableConfig
	}{
		{"datadog", model.CoreSchema, configmock.New},
		{"system-probe", model.SystemProbeSchema, configmock.NewSystemProbe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.newCfg(t)

			sch, err := loadSchema(tc.schemaID)
			require.NoError(t, err, "with `go test`, generate the embedded schema first: dda inv schema.compress")

			data, err := normalizeForSchema(cfg.AllSettings())
			require.NoError(t, err)

			err = sch.Validate(data)
			if err == nil {
				return
			}
			var ve *jsonschema.ValidationError
			require.True(t, errors.As(err, &ve), "unexpected validation error: %s", err)
			var violations []*model.Violation
			collectViolations(ve, nil, data, false, &violations)
			for _, violation := range violations {
				t.Errorf("default value doesn't match the schema: %s", violation.Message)
			}
		})
	}
}
