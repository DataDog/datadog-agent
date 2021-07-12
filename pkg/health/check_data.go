package health

import "fmt"
import "encoding/json"

// CheckData is the data for a health check
type CheckData map[string]interface{}

// JSONString returns a JSON string of the Component
func (c CheckData) JSONString() string {
	b, err := json.Marshal(c)
	if err != nil {
		fmt.Println(err)
		return fmt.Sprintf("{\"error\": \"%s\"}", err.Error())
	}
	return string(b)
}
