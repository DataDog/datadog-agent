package topology

import (
	"encoding/json"
	"fmt"
)

// Relation is a representation of a topology relation
type Relation struct {
	ExternalID string `json:"externalId"`
	SourceID   string `json:"sourceId"`
	TargetID   string `json:"targetId"`
	Type       Type   `json:"type"`
	Data       Data   `json:"data"`
}

// JSONString returns a JSON string of the Relation
func (r Relation) JSONString() string {
	b, err := json.Marshal(r)
	if err != nil {
		fmt.Println(err)
		return fmt.Sprintf("{\"error\": \"%s\"}", err.Error())
	}
	return string(b)
}
