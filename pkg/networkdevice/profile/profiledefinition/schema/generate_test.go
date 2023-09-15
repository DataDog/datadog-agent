// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package schema

import (
	_ "embed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var (
	//go:embed profile_rc_schema.json
	deviceProfileRcConfigJsonschema []byte
)

func TestGenerateJSONSchema(t *testing.T) {
	schemaJSON, err := GenerateJSONSchema()
	require.NoError(t, err)

	assert.Equal(t, string(deviceProfileRcConfigJsonschema), string(schemaJSON))
}
