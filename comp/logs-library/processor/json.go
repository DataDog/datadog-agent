// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const (
	nanoToMillis = 1000000
	hexDigits    = "0123456789abcdef"
)

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

// Encode encodes a message into a JSON byte array.
func (j *jsonEncoder) Encode(msg *message.Message, hostname string) error {
	if msg.State != message.StateRendered {
		return errors.New("message passed to encoder isn't rendered")
	}

	ts := msg.LogTimestamp(j.useContainerTimestamp)
	content := msg.GetContent()
	status := msg.GetStatus()
	service := msg.Origin.Service()
	source := msg.Origin.Source()
	tags := msg.Origin.TransportTags()

	tagsLen := 0
	for _, t := range tags {
		tagsLen += len(t) + 1
	}

	const overhead = len(`{"message":"","status":"","timestamp":,"hostname":"","service":"","ddsource":"","ddtags":""}`) + 20
	buf := make([]byte, 0, overhead+len(content)+len(status)+len(hostname)+len(service)+len(source)+tagsLen)

	buf = append(buf, `{"message":"`...)
	buf = appendJSONBytes(buf, content)
	buf = append(buf, `","status":"`...)
	buf = appendJSONString(buf, status)
	buf = append(buf, `","timestamp":`...)
	buf = strconv.AppendInt(buf, ts.UnixNano()/nanoToMillis, 10)
	buf = append(buf, `,"hostname":"`...)
	buf = appendJSONString(buf, hostname)
	buf = append(buf, `","service":"`...)
	buf = appendJSONString(buf, service)
	buf = append(buf, `","ddsource":"`...)
	buf = appendJSONString(buf, source)
	buf = append(buf, `","ddtags":"`...)
	for i, t := range tags {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = appendJSONString(buf, t)
	}
	buf = append(buf, `"}`...)

	msg.SetEncoded(buf)
	return nil
}

// appendJSONBytes appends b as JSON string content matching encoding/json on ValidUtf8Bytes
// (invalid UTF-8 bytes become literal U+FFFD, HTML chars <, >, & and U+2028/U+2029 are escaped).
func appendJSONBytes(dst, b []byte) []byte {
	start := 0
	for i := 0; i < len(b); {
		if c := b[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			dst = append(dst, b[start:i]...)
			dst = appendEscapedASCII(dst, c)
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, b[start:i]...)
			dst = utf8.AppendRune(dst, utf8.RuneError)
			i++
			start = i
			continue
		}
		if r == 0x2028 || r == 0x2029 {
			dst = append(dst, b[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
			i += size
			start = i
			continue
		}
		i += size
	}
	return append(dst, b[start:]...)
}

// appendJSONString appends s as JSON string content matching encoding/json.Marshal.
func appendJSONString(dst []byte, s string) []byte {
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			dst = appendEscapedASCII(dst, c)
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', 'f', 'f', 'f', 'd')
			i++
			start = i
			continue
		}
		if r == 0x2028 || r == 0x2029 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
			i += size
			start = i
			continue
		}
		i += size
	}
	return append(dst, s[start:]...)
}

func appendEscapedASCII(dst []byte, c byte) []byte {
	switch c {
	case '\\', '"':
		return append(dst, '\\', c)
	case '\b':
		return append(dst, '\\', 'b')
	case '\f':
		return append(dst, '\\', 'f')
	case '\n':
		return append(dst, '\\', 'n')
	case '\r':
		return append(dst, '\\', 'r')
	case '\t':
		return append(dst, '\\', 't')
	default:
		return append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
	}
}
