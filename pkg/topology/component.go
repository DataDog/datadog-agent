package topology

import (
	"encoding/json"
	"fmt"
)

// Data type is used as an alias for the golang map
type Data map[string]interface{}

// Component is a representation of a topology component
type Component struct {
	ExternalID string `json:"externalId"`
	Type       Type   `json:"type"`
	Data       Data   `json:"data"`
}

// JSONString returns a JSON string of the Component
func (c Component) JSONString() string {
	b, err := json.Marshal(c)
	if err != nil {
		fmt.Println(err)
		return fmt.Sprintf("{\"error\": \"%s\"}", err.Error())
	}
	return string(b)
}

// PutNonEmpty adds the value for the given key to the map if the value is not nil
func (d Data) PutNonEmpty(key string, value interface{}) bool {
	if value != nil {
		switch value.(type) {
		case map[string]string:
			if len(value.(map[string]string)) != 0 {
				d[key] = value
			}
		case string:
			if value.(string) != "" {
				d[key] = value
			}
		default:
			d[key] = value
		}
	}

	return true
}
