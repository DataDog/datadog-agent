package remote

import (
	"encoding/json"
	"fmt"
)

type versionCustom struct {
	Version uint64 `json:"version"`
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
	return version.Version, nil
}
