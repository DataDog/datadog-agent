// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

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

	msgContent := msg.GetContent()
	var (
		encoded []byte
		err     error
	)

	if isValidRawString(msgContent) { // fast path
		encoded, err = jsonEncodeFastPath(fastPathPayload{
			content:   msgContent,
			status:    msg.GetStatus(),
			timestamp: ts.UnixNano() / nanoToMillis,
			hostname:  hostname,
			service:   msg.Origin.Service(),
			source:    msg.Origin.Source(),
			tags:      msg.TagsToString(),
		})
	} else { // slow path
		encoded, err = json.Marshal(jsonPayload{
			Message:   toValidUtf8(msgContent),
			Status:    msg.GetStatus(),
			Timestamp: ts.UnixNano() / nanoToMillis,
			Hostname:  hostname,
			Service:   msg.Origin.Service(),
			Source:    msg.Origin.Source(),
			Tags:      msg.TagsToString(),
		})
	}

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}

type fastPathPayload struct {
	content   []byte
	status    string
	timestamp int64
	hostname  string
	service   string
	source    string
	tags      string
}

func jsonEncodeFastPath(payload fastPathPayload) ([]byte, error) {
	var buff bytes.Buffer

	appendRawString := func(key, value string, lastField bool) {
		buff.WriteByte('"')
		buff.WriteString(key)
		buff.WriteString(`":"`)
		buff.WriteString(value)
		buff.WriteByte('"')
		if !lastField {
			buff.WriteByte(',')
		}
	}

	preAllocKeyValue := func(key int, value int) int {
		// quote + key + quote + colon + quote + value + quote
		return 1 + key + 3 + value + 1
	}

	// {} + all fields + (number of fields - 1) commas
	preAlloc := 2 +
		preAllocKeyValue(len("message"), len(payload.content)) +
		preAllocKeyValue(len("status"), len(payload.status)) +
		preAllocKeyValue(len("timestamp"), 19) + // max int64 in string is 19 chars
		preAllocKeyValue(len("hostname"), len(payload.hostname)) +
		preAllocKeyValue(len("service"), len(payload.service)) +
		preAllocKeyValue(len("ddsource"), len(payload.source)) +
		preAllocKeyValue(len("ddtags"), len(payload.tags)) +
		6
	buff.Grow(preAlloc)

	// header
	buff.WriteByte('{')

	// message
	buff.WriteString(`"message":`)
	buff.WriteByte('"')
	buff.Write(payload.content)
	buff.WriteByte('"')
	buff.WriteByte(',')

	// status
	appendRawString("status", payload.status, false)

	// timestamp
	buff.WriteString(`"timestamp":`)
	tmp := buff.AvailableBuffer()
	tmp = strconv.AppendInt(tmp, payload.timestamp, 10)
	buff.Write(tmp)
	buff.WriteByte(',')

	// hostname
	appendRawString("hostname", payload.hostname, false)
	// service
	appendRawString("service", payload.service, false)
	// source
	appendRawString("ddsource", payload.source, false)
	// tags
	appendRawString("ddtags", payload.tags, true)

	// footer
	buff.WriteByte('}')

	return buff.Bytes(), nil
}

func isValidRawString(s []byte) bool {
	if !utf8.Valid(s) {
		return false
	}

	for _, b := range s {
		if b < 0x20 || b == '"' || b == '\\' || b == '<' || b == '>' || b == '&' {
			return false
		}
	}

	return true
}
