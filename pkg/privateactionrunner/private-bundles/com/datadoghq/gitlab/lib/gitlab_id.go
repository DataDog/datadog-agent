package lib

import (
	"encoding/json"
	"fmt"
)

type GitlabID string

func (gid *GitlabID) UnmarshalJSON(data []byte) error {
	// First try to unmarshal a json.Number: 123 or "123"
	var id json.Number
	err := json.Unmarshal(data, &id)
	if err == nil {
		*gid = GitlabID(id.String())
		return nil
	}

	// If that fails, try to get the string value: "group_name/project_name"
	var sid string
	err = json.Unmarshal(data, &sid)
	if err != nil {
		return fmt.Errorf("invalid gitlab ID: expecting string or number, got: %s", data)
	}
	*gid = GitlabID(sid)
	return nil
}

func (gid *GitlabID) String() string {
	return string(*gid)
}
