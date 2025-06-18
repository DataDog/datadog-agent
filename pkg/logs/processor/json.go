// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
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

	encoded, err := jsonEncodeFastPath(fastPathPayload{
		content:   msgContent,
		status:    msg.GetStatus(),
		timestamp: ts.UnixNano() / nanoToMillis,
		hostname:  hostname,
		service:   msg.Origin.Service(),
		source:    msg.Origin.Source(),
		tags:      msg.TagsToString(),
	})

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
	writeEscapedString(&buff, payload.content)
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

func isHTMLJSONSafe(b byte) bool {
	// Check if the byte is a valid HTML JSON safe character
	return b >= 0x20 && b != '"' && b != '\\' && b != '<' && b != '>' && b != '&'
}

func writeEscapedString(buff *bytes.Buffer, src []byte) {
	const hex = "0123456789abcdef"

	buff.WriteByte('"')
	start := 0
	for i := 0; i < len(src); {
		if b := src[i]; b < utf8.RuneSelf {
			if isHTMLJSONSafe(b) {
				i++
				continue
			}
			buff.Write(src[start:i])
			switch b {
			case '\\', '"':
				buff.WriteByte('\\')
				buff.WriteByte(b)
			case '\b':
				buff.WriteByte('\\')
				buff.WriteByte('b')
			case '\f':
				buff.WriteByte('\\')
				buff.WriteByte('f')
			case '\n':
				buff.WriteByte('\\')
				buff.WriteByte('n')
			case '\r':
				buff.WriteByte('\\')
				buff.WriteByte('r')
			case '\t':
				buff.WriteByte('\\')
				buff.WriteByte('t')
			default:
				// This encodes bytes < 0x20 except for \b, \f, \n, \r and \t.
				// If escapeHTML is set, it also escapes <, >, and &
				// because they can lead to security holes when
				// user-controlled strings are rendered into JSON
				// and served to some browsers.
				buff.Write([]byte{'\\', 'u', '0', '0', hex[b>>4], hex[b&0xF]})
			}
			i++
			start = i
			continue
		}
		// TODO(https://go.dev/issue/56948): Use generic utf8 functionality.
		// For now, cast only a small portion of byte slices to a string
		// so that it can be stack allocated. This slows down []byte slightly
		// due to the extra copy, but keeps string performance roughly the same.
		n := len(src) - i
		if n > utf8.UTFMax {
			n = utf8.UTFMax
		}
		c, size := utf8.DecodeRuneInString(string(src[i : i+n]))
		if c == utf8.RuneError && size == 1 {
			buff.Write(src[start:i])
			buff.WriteString(`\ufffd`)
			i += size
			start = i
			continue
		}
		// U+2028 is LINE SEPARATOR.
		// U+2029 is PARAGRAPH SEPARATOR.
		// They are both technically valid characters in JSON strings,
		// but don't work in JSONP, which has to be evaluated as JavaScript,
		// and can lead to security holes there. It is valid JSON to
		// escape them, so we do so unconditionally.
		// See https://en.wikipedia.org/wiki/JSON#Safety.
		if c == '\u2028' || c == '\u2029' {
			buff.Write(src[start:i])
			buff.Write([]byte{'\\', 'u', '2', '0', '2', hex[c&0xF]})
			i += size
			start = i
			continue
		}
		i += size
	}
	buff.Write(src[start:])
	buff.WriteByte('"')
}
