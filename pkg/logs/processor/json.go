// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const nanoToMillis = 1000000

// JSONEncoder is a shared json encoder.
var JSONEncoder Encoder = &jsonEncoder{}

// JSONPayload is a shared JSON representation of a message
var JSONPayload = jsonPayload{}

// jsonEncoder transforms a message into a JSON byte array.
type jsonEncoder struct{}

// JSON representation of a message.
type jsonPayload struct {
	Message   string `json:"message"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
	Hostname  string `json:"hostname"`
	Service   string `json:"service"`
	Source    string `json:"ddsource"`
	Tags      string `json:"ddtags"`
}

// Encode encodes a message into a JSON byte array.
func (j *jsonEncoder) Encode(msg *message.Message, hostname string) error {
	if msg.State != message.StateRendered {
		return fmt.Errorf("message passed to encoder isn't rendered")
	}

	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}

	encoded, err := encodeInner(jsonPayload{
		Message:   toValidUtf8(msg.GetContent()),
		Status:    msg.GetStatus(),
		Timestamp: ts.UnixNano() / nanoToMillis,
		Hostname:  hostname,
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      msg.TagsToString(),
	})

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}

// func encodeInner(payload jsonPayload) ([]byte, error) {
// 	return json.Marshal(payload)
// }

// func encodeInner(payload jsonPayload) ([]byte, error) {
// 	var sb strings.Builder
// 	sb.Grow(255) // arbitrary constant, unclear if too big or too small

// 	sb.WriteByte('{')
// 	// "message": <string>
// 	sb.WriteString(`"message":`)
// 	escapedString(&sb, payload.Message)
// 	sb.WriteByte(',')

// 	// "status": <string>
// 	sb.WriteString(`"status":`)
// 	escapedString(&sb, payload.Status)
// 	sb.WriteByte(',')

// 	// "timestamp": <number>
// 	sb.WriteString(`"timestamp":`)
// 	sb.WriteString(strconv.FormatInt(payload.Timestamp, 10))
// 	sb.WriteByte(',')

// 	// "hostname": <string>
// 	sb.WriteString(`"hostname":`)
// 	escapedString(&sb, payload.Hostname)
// 	sb.WriteByte(',')

// 	// "service": <string>
// 	sb.WriteString(`"service":`)
// 	escapedString(&sb, payload.Service)
// 	sb.WriteByte(',')

// 	// "ddsource": <string>
// 	sb.WriteString(`"ddsource":`)
// 	escapedString(&sb, payload.Source)
// 	sb.WriteByte(',')

// 	// "ddtags": <string>
// 	sb.WriteString(`"ddtags":`)
// 	escapedString(&sb, payload.Tags)

// 	sb.WriteByte('}')

// 	return []byte(sb.String()), nil
// }

// func escapedString(sb *strings.Builder, s string) {
// 	sb.WriteByte('"')
// 	for i := 0; i < len(s); i++ {
// 		c := s[i]
// 		switch c {
// 		case '\\':
// 			sb.WriteString(`\\`)
// 		case '"':
// 			sb.WriteString(`\"`)
// 		case '\n':
// 			sb.WriteString(`\n`)
// 		case '\r':
// 			sb.WriteString(`\r`)
// 		case '\t':
// 			sb.WriteString(`\t`)
// 		default:
//			// TODO chars <0x20
// 			sb.WriteByte(c)
// 		}
// 	}
// 	sb.WriteByte('"')
// }

func encodeInner(payload jsonPayload) ([]byte, error) {
	b := make([]byte, 0, 256) // arbitrary constant, unclear if too big or too small

	b = append(b, '{')

	b = append(b, `"message":`...)
	b = escapedString(b, payload.Message)
	b = append(b, ',')

	b = append(b, `"status":`...)
	b = escapedString(b, payload.Status)
	b = append(b, ',')

	b = append(b, `"timestamp":`...)
	b = strconv.AppendInt(b, payload.Timestamp, 10)
	b = append(b, ',')

	b = append(b, `"hostname":`...)
	b = escapedString(b, payload.Hostname)
	b = append(b, ',')

	b = append(b, `"service":`...)
	b = escapedString(b, payload.Service)
	b = append(b, ',')

	b = append(b, `"ddsource":`...)
	b = escapedString(b, payload.Source)
	b = append(b, ',')

	b = append(b, `"ddtags":`...)
	b = escapedString(b, payload.Tags)

	b = append(b, '}')
	return b, nil
}

func escapedString(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b = append(b, `\\`...)
		case '"':
			b = append(b, `\"`...)
		case '\n':
			b = append(b, `\n`...)
		case '\r':
			b = append(b, `\r`...)
		case '\t':
			b = append(b, `\t`...)
		default:
			// TODO chars <0x20
			b = append(b, s[i])
		}
	}
	b = append(b, '"')
	return b
}
