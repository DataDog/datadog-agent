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

func TestGenerateJsonSchema(t *testing.T) {
	schemaJSON, err := GenerateJsonSchema()
	require.NoError(t, err)

	assert.Equal(t, string(deviceProfileRcConfigJsonschema), string(schemaJSON))
}
