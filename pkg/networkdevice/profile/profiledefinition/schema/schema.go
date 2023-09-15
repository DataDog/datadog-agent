package schema

import _ "embed"

var (
	//go:embed profile_rc_schema.json
	DeviceProfileRcConfigJsonschema []byte
)
