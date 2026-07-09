// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"strconv"
	"sync"

	jsoniter "github.com/json-iterator/go"
)

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
	msg         string
	syslog      SyslogFields
	siem        *SIEMFields
	debugRender bool
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
// parsed SyslogMessage. CEF/LEEF headers in the message body are always
// detected and extracted into the siem field. The caller (parser.go)
// overwrites msg with the full original log line after construction.
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
	}
	return sc
}

// Render produces the message content for downstream encoding. When
// debugRender is false, the raw message string is returned as-is. When
// true, a JSON object with "message", "syslog", and optionally "siem"
// keys is produced via pooled jsoniter.Stream serialization.
func (s *SyslogStructuredContent) Render() ([]byte, error) {
	if !s.debugRender {
		return []byte(s.msg), nil
	}

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

// isAbsentField reports whether a syslog string header field carries no real
// value. Parsers encode an absent field either as the empty string or as the
// nilvalue sentinel "-" (RFC 5424 NILVALUE, and the placeholder used by
// parseBSDNoTimestamp); both must be treated as "not found" so remap_source
// rules never match on a hollow "-".
func isAbsentField(v string) bool {
	return v == "" || v == nilvalue
}

func (f *SyslogFields) getAttribute(field string) (string, bool) {
	top, rest, hasDot := splitFirst(field)
	switch top {
	case "timestamp":
		if isAbsentField(f.Timestamp) || hasDot {
			return "", false
		}
		return f.Timestamp, true
	case "hostname":
		if isAbsentField(f.Hostname) || hasDot {
			return "", false
		}
		return f.Hostname, true
	case "appname":
		if isAbsentField(f.AppName) || hasDot {
			return "", false
		}
		return f.AppName, true
	case "procid":
		if isAbsentField(f.ProcID) || hasDot {
			return "", false
		}
		return f.ProcID, true
	case "msgid":
		if isAbsentField(f.MsgID) || hasDot {
			return "", false
		}
		return f.MsgID, true
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
		val, ok := params[unescapeSegment(paramName)]
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
		val, ok := f.Extension[unescapeSegment(rest)]
		return val, ok
	default:
		return "", false
	}
}

// splitFirst splits s at the first unescaped dot. Backslash escapes are
// supported: \. is a literal dot, \\ is a literal backslash. The first
// segment is returned unescaped; rest is left in escaped form for
// downstream splitFirst calls.
func splitFirst(s string) (first, rest string, hasDot bool) {
	var seg []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '.':
				seg = append(seg, '.')
			case '\\':
				seg = append(seg, '\\')
			default:
				seg = append(seg, '\\', s[i+1])
			}
			i++
			continue
		}
		if s[i] == '.' {
			return string(seg), s[i+1:], true
		}
		seg = append(seg, s[i])
	}
	return string(seg), "", false
}

// unescapeSegment resolves \. and \\ in a terminal path segment that will
// be used directly as a map key (not passed back through splitFirst).
func unescapeSegment(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '.':
				b = append(b, '.')
			case '\\':
				b = append(b, '\\')
			default:
				b = append(b, '\\', s[i+1])
			}
			i++
			continue
		}
		b = append(b, s[i])
	}
	return string(b)
}

// writeTo writes the syslog fields as a JSON object to the stream.
// Field presence: severity/facility omitted when Pri < 0, version omitted
// for BSD, structured_data omitted when nil.
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
// Field presence: name omitted for LEEF, severity omitted for LEEF,
// extension omitted when empty.
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
