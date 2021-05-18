package health

import "fmt"
import "encoding/json"

// Stream is a representation of a health stream for health synchronization
type Stream struct {
	Urn       string `json:"urn"`
	SubStream string `json:"sub_stream,omitempty"`
}

// GoString prints as string, can also be used in maps
func (i *Stream) GoString() string {
	b, err := json.Marshal(i)
	if err != nil {
		fmt.Println(err)
		return fmt.Sprintf("{\"error\": \"%s\"}", err.Error())
	}
	return string(b)
}
