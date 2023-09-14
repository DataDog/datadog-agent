package schema

import (
	_ "embed"
	"encoding/json"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
	"testing"
)

var (
	//go:embed profile_rc_schema.json
	deviceProfileRcConfigJsonschema []byte
)

func Test_DeviceProfileRcConfigJsonSchema(t *testing.T) {
	// language=json
	instanceJson := `{
	"profile_definition": {
		"name": "my-profile"
	}
}`

	err := assertAgainstSchema(t, instanceJson)

	require.NoError(t, err)
}

func assertAgainstSchema(t *testing.T, instanceJson string) error {
	sch, err := jsonschema.CompileString("schema.json", string(deviceProfileRcConfigJsonschema))
	require.NoError(t, err)

	var v interface{}
	err = json.Unmarshal([]byte(instanceJson), &v)
	require.NoError(t, err)

	err = sch.Validate(v)
	return err
}
