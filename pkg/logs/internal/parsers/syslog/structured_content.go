// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	jsoniter "github.com/json-iterator/go"
)

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

var syslogJSONConfig = jsoniter.Config{
	EscapeHTML:                    false,
	ObjectFieldMustBeSimpleString: true,
}.Froze()

var renderStreamPool = sync.Pool{
	New: func() interface{} {
		return jsoniter.NewStream(syslogJSONConfig, nil, 512)
	},
}

// SyslogStructuredContent is a high-performance replacement for
// BasicStructuredContent that stores syslog fields in typed Go structs
// instead of map[string]interface{}. Render() uses jsoniter.Stream to
// write JSON directly without reflection.
type SyslogStructuredContent struct {
	msg    string
	syslog SyslogFields
	siem   *SIEMFields
}

// SyslogFields holds the parsed syslog metadata. Severity and Facility
// are -1 when Pri is absent (e.g. BSD file input without PRI header).
type SyslogFields struct {
	Timestamp      string
	Hostname       string
	AppName        string
	ProcID         string
	MsgID          string
	Severity       int
	Facility       int
	Version        string
	StructuredData map[string]map[string]string
}

// SIEMFields holds normalized CEF/LEEF header and extension data.
type SIEMFields struct {
	Format        string
	Version       string
	DeviceVendor  string
	DeviceProduct string
	DeviceVersion string
	EventID       string
	Name          string
	Severity      string
	Extension     map[string]string
}

// NewSyslogStructuredContent constructs a SyslogStructuredContent from a
// parsed SyslogMessage. If CEF/LEEF is detected in the message body, the
// siem field is populated and message is cleared.
func NewSyslogStructuredContent(parsed SyslogMessage) *SyslogStructuredContent {
	sc := &SyslogStructuredContent{
		msg: string(parsed.Msg),
		syslog: SyslogFields{
			Timestamp:      parsed.Timestamp,
			Hostname:       parsed.Hostname,
			AppName:        parsed.AppName,
			ProcID:         parsed.ProcID,
			MsgID:          parsed.MsgID,
			Version:        parsed.Version,
			StructuredData: parsed.StructuredData,
		},
	}
	if parsed.Pri >= 0 {
		sc.syslog.Severity = parsed.Pri % 8
		sc.syslog.Facility = parsed.Pri / 8
	} else {
		sc.syslog.Severity = -1
		sc.syslog.Facility = -1
	}

	if header, ext, _, ok := ParseCEFLEEF(parsed.Msg); ok {
		sc.siem = &SIEMFields{
			Format:        header.Format,
			Version:       header.Version,
			DeviceVendor:  header.DeviceVendor,
			DeviceProduct: header.DeviceProduct,
			DeviceVersion: header.DeviceVersion,
			EventID:       header.EventID,
			Name:          header.Name,
			Severity:      header.Severity,
			Extension:     ext,
		}
		sc.msg = ""
	}
	return sc
}

// Render produces JSON bytes matching the same schema as BasicStructuredContent:
//
//	{"message":"...","syslog":{...},"siem":{...}}
//
// Uses a pooled jsoniter.Stream for zero-reflection serialization.
// The returned slice is an independent copy safe for concurrent use.
func (s *SyslogStructuredContent) Render() ([]byte, error) {
	stream := renderStreamPool.Get().(*jsoniter.Stream)
	stream.Reset(nil)

	stream.WriteObjectStart()

	stream.WriteObjectField("message")
	stream.WriteString(s.msg)

	stream.WriteMore()
	stream.WriteObjectField("syslog")
	s.syslog.writeTo(stream)

	if s.siem != nil {
		stream.WriteMore()
		stream.WriteObjectField("siem")
		s.siem.writeTo(stream)
	}

	stream.WriteObjectEnd()

	if stream.Error != nil {
		err := stream.Error
		renderStreamPool.Put(stream)
		return nil, err
	}

	buf := stream.Buffer()
	result := make([]byte, len(buf))
	copy(result, buf)
	renderStreamPool.Put(stream)
	return result, nil
}

// EncodeFull produces the complete transport-envelope JSON in a single pass.
// It calls Render() internally to obtain the inner JSON, then wraps it using
// direct []byte append with a tight escape loop.
func (s *SyslogStructuredContent) EncodeFull(
	status string, timestamp int64,
	hostname, service, source, tags string,
) ([]byte, error) {
	rendered, err := s.Render()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 0, len(rendered)+len(rendered)/8+256)

	buf = append(buf, `{"message":"`...)
	buf = appendEscapedBytes(buf, rendered)
	buf = append(buf, `","status":"`...)
	buf = appendEscapedString(buf, status)
	buf = append(buf, `","timestamp":`...)
	buf = strconv.AppendInt(buf, timestamp, 10)
	buf = append(buf, `,"hostname":"`...)
	buf = appendEscapedString(buf, hostname)
	buf = append(buf, `","service":"`...)
	buf = appendEscapedString(buf, service)
	buf = append(buf, `","ddsource":"`...)
	buf = appendEscapedString(buf, source)
	buf = append(buf, `","ddtags":"`...)
	buf = appendEscapedString(buf, tags)
	buf = append(buf, `"}`...)

	return buf, nil
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

// GetContent returns the message body for processing rules (scrubbing).
func (s *SyslogStructuredContent) GetContent() []byte {
	return []byte(s.msg)
}

// SetContent updates the message body after processing rules (scrubbing).
func (s *SyslogStructuredContent) SetContent(content []byte) {
	s.msg = string(content)
}

// GetAttribute implements dot-path attribute lookup for processing rules
// (e.g. RemapAttributeToSource). Supports paths like "syslog.hostname",
// "syslog.severity", "siem.device_vendor", or "message".
func (s *SyslogStructuredContent) GetAttribute(path string) (string, bool) {
	top, rest, hasDot := splitFirst(path)

	switch top {
	case "message":
		if hasDot {
			return "", false
		}
		return s.msg, true

	case "syslog":
		if !hasDot {
			return "", false
		}
		return s.syslog.getAttribute(rest)

	case "siem":
		if s.siem == nil || !hasDot {
			return "", false
		}
		return s.siem.getAttribute(rest)

	default:
		return "", false
	}
}

func (f *SyslogFields) getAttribute(field string) (string, bool) {
	top, rest, hasDot := splitFirst(field)
	switch top {
	case "timestamp":
		return f.Timestamp, !hasDot
	case "hostname":
		return f.Hostname, !hasDot
	case "appname":
		return f.AppName, !hasDot
	case "procid":
		return f.ProcID, !hasDot
	case "msgid":
		return f.MsgID, !hasDot
	case "severity":
		if f.Severity < 0 || hasDot {
			return "", false
		}
		return strconv.Itoa(f.Severity), true
	case "facility":
		if f.Facility < 0 || hasDot {
			return "", false
		}
		return strconv.Itoa(f.Facility), true
	case "version":
		if f.Version == "" || hasDot {
			return "", false
		}
		return f.Version, true
	case "structured_data":
		if f.StructuredData == nil {
			return "", false
		}
		if !hasDot {
			return "", false
		}
		sdID, paramName, hasParam := splitFirst(rest)
		if !hasParam {
			return "", false
		}
		params, ok := f.StructuredData[sdID]
		if !ok {
			return "", false
		}
		val, ok := params[paramName]
		return val, ok
	default:
		return "", false
	}
}

func (f *SIEMFields) getAttribute(field string) (string, bool) {
	top, rest, hasDot := splitFirst(field)
	switch top {
	case "format":
		return f.Format, !hasDot
	case "version":
		return f.Version, !hasDot
	case "device_vendor":
		return f.DeviceVendor, !hasDot
	case "device_product":
		return f.DeviceProduct, !hasDot
	case "device_version":
		return f.DeviceVersion, !hasDot
	case "event_id":
		return f.EventID, !hasDot
	case "name":
		if f.Name == "" || hasDot {
			return "", false
		}
		return f.Name, true
	case "severity":
		if f.Severity == "" || hasDot {
			return "", false
		}
		return f.Severity, true
	case "extension":
		if f.Extension == nil {
			return "", false
		}
		if !hasDot {
			return "", false
		}
		val, ok := f.Extension[rest]
		return val, ok
	default:
		return "", false
	}
}

func splitFirst(s string) (first, rest string, hasDot bool) {
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}

// writeTo writes the syslog fields as a JSON object to the stream.
// Field presence matches BuildSyslogFields: severity/facility omitted when
// Pri < 0, version omitted for BSD, structured_data omitted when nil.
func (f *SyslogFields) writeTo(stream *jsoniter.Stream) {
	stream.WriteObjectStart()

	stream.WriteObjectField("timestamp")
	stream.WriteString(f.Timestamp)

	stream.WriteMore()
	stream.WriteObjectField("hostname")
	stream.WriteString(f.Hostname)

	stream.WriteMore()
	stream.WriteObjectField("appname")
	stream.WriteString(f.AppName)

	stream.WriteMore()
	stream.WriteObjectField("procid")
	stream.WriteString(f.ProcID)

	stream.WriteMore()
	stream.WriteObjectField("msgid")
	stream.WriteString(f.MsgID)

	if f.Severity >= 0 {
		stream.WriteMore()
		stream.WriteObjectField("severity")
		stream.WriteInt(f.Severity)

		stream.WriteMore()
		stream.WriteObjectField("facility")
		stream.WriteInt(f.Facility)
	}

	if f.Version != "" {
		stream.WriteMore()
		stream.WriteObjectField("version")
		stream.WriteString(f.Version)
	}

	if f.StructuredData != nil {
		stream.WriteMore()
		stream.WriteObjectField("structured_data")
		writeSDMap(stream, f.StructuredData)
	}

	stream.WriteObjectEnd()
}

// writeTo writes the SIEM fields as a JSON object to the stream.
// Field presence matches BuildSIEMFields: name omitted for LEEF,
// severity omitted for LEEF, extension omitted when empty.
func (f *SIEMFields) writeTo(stream *jsoniter.Stream) {
	stream.WriteObjectStart()

	stream.WriteObjectField("format")
	stream.WriteString(f.Format)

	stream.WriteMore()
	stream.WriteObjectField("version")
	stream.WriteString(f.Version)

	stream.WriteMore()
	stream.WriteObjectField("device_vendor")
	stream.WriteString(f.DeviceVendor)

	stream.WriteMore()
	stream.WriteObjectField("device_product")
	stream.WriteString(f.DeviceProduct)

	stream.WriteMore()
	stream.WriteObjectField("device_version")
	stream.WriteString(f.DeviceVersion)

	stream.WriteMore()
	stream.WriteObjectField("event_id")
	stream.WriteString(f.EventID)

	if f.Name != "" {
		stream.WriteMore()
		stream.WriteObjectField("name")
		stream.WriteString(f.Name)
	}

	if f.Severity != "" {
		stream.WriteMore()
		stream.WriteObjectField("severity")
		stream.WriteString(f.Severity)
	}

	if len(f.Extension) > 0 {
		stream.WriteMore()
		stream.WriteObjectField("extension")
		stream.WriteObjectStart()
		first := true
		for k, v := range f.Extension {
			if !first {
				stream.WriteMore()
			}
			stream.WriteObjectField(k)
			stream.WriteString(v)
			first = false
		}
		stream.WriteObjectEnd()
	}

	stream.WriteObjectEnd()
}

func writeSDMap(stream *jsoniter.Stream, sd map[string]map[string]string) {
	stream.WriteObjectStart()
	first := true
	for sdID, params := range sd {
		if !first {
			stream.WriteMore()
		}
		stream.WriteObjectField(sdID)
		stream.WriteObjectStart()
		firstParam := true
		for k, v := range params {
			if !firstParam {
				stream.WriteMore()
			}
			stream.WriteObjectField(k)
			stream.WriteString(v)
			firstParam = false
		}
		stream.WriteObjectEnd()
		first = false
	}
	stream.WriteObjectEnd()
}
