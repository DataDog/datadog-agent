package remote

import (
	"encoding/json"
	"fmt"
)

type versionCustom struct {
	Version *uint64 `json:"v"`
}

func targetVersion(custom *json.RawMessage) (uint64, error) {
	if custom == nil {
		return 0, fmt.Errorf("custom is nil")
	}
	var version versionCustom
	err := json.Unmarshal(*custom, &version)
	if err != nil {
		return 0, err
	}
	if version.Version == nil {
		return 0, fmt.Errorf("custom.v is not defined, could not get target version")
	}
	return *version.Version, nil
}
