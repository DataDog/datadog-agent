package schema

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/invopop/jsonschema"
)

func GenerateJsonSchema() ([]byte, error) {
	schema := jsonschema.Reflect(&profiledefinition.DeviceProfileRcConfig{})
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	schemaJSON = append(schemaJSON, byte('\n'))
	return schemaJSON, nil
}
