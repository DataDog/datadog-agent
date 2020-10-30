package serverless

import (
	"encoding/json"
	"fmt"
	"time"
)

// LogMessage is a log message sent by the AWS API.
type LogMessage struct {
	Time   time.Time `json:"time"`
	Type   string    `json:"type"`
	Record string    `json:"record"`
}

func (l *LogMessage) UnmarshalJSON(data []byte) error {
	var j map[string]interface{}

	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}

	var typ string
	var ok bool

	if typ, ok = j["type"].(string); !ok {
		return fmt.Errorf("Malformed log message")
	}

	switch typ {
	case "extension":
		fallthrough
	case "function":
		// TODO(remy): l.Time
		l.Type = typ
		l.Record = j["record"].(string)
	default:
		// we're not parsing this kind of message
	}

	return nil
}
