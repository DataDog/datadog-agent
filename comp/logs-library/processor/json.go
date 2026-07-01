// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
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
type jsonEncoder struct {
	useContainerTimestamp bool
}

// NewJSONEncoder returns a JSON encoder configured to optionally use container-provided timestamps.
func NewJSONEncoder(useContainerTimestamp bool) Encoder {
	return &jsonEncoder{useContainerTimestamp: useContainerTimestamp}
}

// JSON representation of a message.
type jsonPayload struct {
	Message   ValidUtf8Bytes `json:"message"`
	Status    string         `json:"status"`
	Timestamp int64          `json:"timestamp"`
	Hostname  string         `json:"hostname"`
	Service   string         `json:"service"`
	Source    string         `json:"ddsource"`
	Tags      string         `json:"ddtags"`
}

const hexDigits = "0123456789abcdef"

// needsEscape marks bytes that require escaping when embedded in a JSON string.
// true for: control chars (0x00-0x1F), '"' (0x22), '\\' (0x5C).
var needsEscape [256]bool

func init() {
	for i := 0; i < 0x20; i++ {
		needsEscape[i] = true
	}
	needsEscape['"'] = true
	needsEscape['\\'] = true
}

// appendEscapedBytes appends b to buf as the contents of a JSON string
// (without surrounding quotes), escaping characters per RFC 8259 and
// replacing invalid UTF-8 with U+FFFD to match ValidUtf8Bytes semantics.
func appendEscapedBytes(buf, b []byte) []byte {
	start := 0
	for i := 0; i < len(b); {
		c := b[i]
		if !needsEscape[c] && c < utf8.RuneSelf {
			i++
			continue
		}
		if c < utf8.RuneSelf {
			buf = append(buf, b[start:i]...)
			switch c {
			case '"':
				buf = append(buf, '\\', '"')
			case '\\':
				buf = append(buf, '\\', '\\')
			case '\n':
				buf = append(buf, '\\', 'n')
			case '\r':
				buf = append(buf, '\\', 'r')
			case '\t':
				buf = append(buf, '\\', 't')
			default:
				buf = append(buf, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			buf = append(buf, b[start:i]...)
			buf = append(buf, "\ufffd"...)
			i++
			start = i
			continue
		}
		i += size
	}
	return append(buf, b[start:]...)
}

// appendEscapedString is the string variant for the envelope's simple
// fields (status, hostname, etc.). These are almost always pure ASCII
// with no characters that need escaping, so the fast path is just append.
func appendEscapedString(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if needsEscape[c] || c >= utf8.RuneSelf {
			return appendEscapedBytes(buf, []byte(s))
		}
	}
	return append(buf, s...)
}

// Encode encodes a message into a JSON byte array.
func (j *jsonEncoder) Encode(msg *message.Message, hostname string) error {
	ts := msg.ServerlessExtra.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
		if msg.ParsingExtra.Timestamp != "" && j.useContainerTimestamp {
			if logTime, err := time.Parse(time.RFC3339Nano, msg.ParsingExtra.Timestamp); err == nil {
				ts = logTime
			}
		}
	}
	tsMillis := ts.UnixNano() / nanoToMillis

	content, err := msg.RenderMessage()
	if err != nil {
		return fmt.Errorf("can't render the message: %v", err)
	}

	status := msg.GetStatus()
	service := msg.Origin.Service()
	source := msg.Origin.Source()
	tags := msg.TagsToString()

	buf := make([]byte, 0, len(content)+len(content)/8+256)
	buf = append(buf, `{"message":"`...)
	buf = appendEscapedBytes(buf, content)
	buf = append(buf, `","status":"`...)
	buf = appendEscapedString(buf, status)
	buf = append(buf, `","timestamp":`...)
	buf = strconv.AppendInt(buf, tsMillis, 10)
	buf = append(buf, `,"hostname":"`...)
	buf = appendEscapedString(buf, hostname)
	buf = append(buf, `","service":"`...)
	buf = appendEscapedString(buf, service)
	buf = append(buf, `","ddsource":"`...)
	buf = appendEscapedString(buf, source)
	buf = append(buf, `","ddtags":"`...)
	buf = appendEscapedString(buf, tags)
	buf = append(buf, `"}`...)

	msg.SetEncoded(buf)
	return nil
}
