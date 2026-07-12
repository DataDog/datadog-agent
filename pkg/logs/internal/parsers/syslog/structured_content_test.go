// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// ---------------------------------------------------------------------------
// Render() debug JSON golden output
// ---------------------------------------------------------------------------

// renderDebugJSON renders parsed with debugRender enabled and unmarshals the
// resulting JSON into a generic map for structural comparison. Numeric fields
// decode to float64, matching encoding/json semantics.
func renderDebugJSON(t *testing.T, parsed SyslogMessage) map[string]interface{} {
	t.Helper()
	sc := NewSyslogStructuredContent(parsed)
	sc.debugRender = true
	got, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(got, &data))
	return data
}

func TestRenderDebugJSON_RFC5424(t *testing.T) {
	got := renderDebugJSON(t, SyslogMessage{
		Pri:       165,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "mymachine.example.com",
		AppName:   "evntslog",
		ProcID:    "-",
		MsgID:     "ID47",
		StructuredData: map[string]map[string]string{
			"exampleSDID@32473": {"iut": "3", "eventSource": "Application", "eventID": "1011"},
		},
		Msg: []byte("An application event log entry"),
	})

	assert.Equal(t, map[string]interface{}{
		"message": "An application event log entry",
		"syslog": map[string]interface{}{
			"timestamp": "2003-10-11T22:14:15.003Z",
			"hostname":  "mymachine.example.com",
			"appname":   "evntslog",
			"procid":    "-",
			"msgid":     "ID47",
			"severity":  float64(5),  // 165 % 8
			"facility":  float64(20), // 165 / 8
			"version":   "1",
			"structured_data": map[string]interface{}{
				"exampleSDID@32473": map[string]interface{}{
					"iut": "3", "eventSource": "Application", "eventID": "1011",
				},
			},
		},
	}, got)
}

func TestRenderDebugJSON_RFC5424_Short(t *testing.T) {
	got := renderDebugJSON(t, SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "host",
		AppName:   "app",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("short"),
	})

	assert.Equal(t, map[string]interface{}{
		"message": "short",
		"syslog": map[string]interface{}{
			"timestamp": "2003-10-11T22:14:15.003Z",
			"hostname":  "host",
			"appname":   "app",
			"procid":    "-",
			"msgid":     "-",
			"severity":  float64(6), // 14 % 8
			"facility":  float64(1), // 14 / 8
			"version":   "1",
		},
	}, got)
}

func TestRenderDebugJSON_BSD(t *testing.T) {
	got := renderDebugJSON(t, SyslogMessage{
		Pri:       34,
		Timestamp: "Oct 11 22:14:15",
		Hostname:  "mymachine",
		AppName:   "su",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("'su root' failed for lonvick on /dev/pts/8"),
	})

	// BSD: no version, no structured_data.
	assert.Equal(t, map[string]interface{}{
		"message": "'su root' failed for lonvick on /dev/pts/8",
		"syslog": map[string]interface{}{
			"timestamp": "Oct 11 22:14:15",
			"hostname":  "mymachine",
			"appname":   "su",
			"procid":    "-",
			"msgid":     "-",
			"severity":  float64(2), // 34 % 8
			"facility":  float64(4), // 34 / 8
		},
	}, got)
}

func TestRenderDebugJSON_NoPri(t *testing.T) {
	got := renderDebugJSON(t, SyslogMessage{
		Pri:       -1,
		Timestamp: "Oct 11 22:14:15",
		Hostname:  "mymachine",
		AppName:   "syslogd",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("restart"),
	})

	// Pri < 0: severity and facility omitted.
	assert.Equal(t, map[string]interface{}{
		"message": "restart",
		"syslog": map[string]interface{}{
			"timestamp": "Oct 11 22:14:15",
			"hostname":  "mymachine",
			"appname":   "syslogd",
			"procid":    "-",
			"msgid":     "-",
		},
	}, got)
}

func TestRenderDebugJSON_NILVALUE(t *testing.T) {
	got := renderDebugJSON(t, SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "-",
		Hostname:  "-",
		AppName:   "-",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("test"),
	})

	assert.Equal(t, map[string]interface{}{
		"message": "test",
		"syslog": map[string]interface{}{
			"timestamp": "-",
			"hostname":  "-",
			"appname":   "-",
			"procid":    "-",
			"msgid":     "-",
			"severity":  float64(6),
			"facility":  float64(1),
			"version":   "1",
		},
	}, got)
}

func TestRenderDebugJSON_CEF(t *testing.T) {
	got := renderDebugJSON(t, SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "host",
		AppName:   "app",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte(`CEF:0|Security|Firewall|1.0|100|Attack|10|src=1.2.3.4 dst=5.6.7.8`),
	})

	assert.Equal(t, map[string]interface{}{
		"message": `CEF:0|Security|Firewall|1.0|100|Attack|10|src=1.2.3.4 dst=5.6.7.8`,
		"syslog": map[string]interface{}{
			"timestamp": "2003-10-11T22:14:15.003Z",
			"hostname":  "host",
			"appname":   "app",
			"procid":    "-",
			"msgid":     "-",
			"severity":  float64(6),
			"facility":  float64(1),
			"version":   "1",
		},
		"siem": map[string]interface{}{
			"format":         "CEF",
			"version":        "0",
			"device_vendor":  "Security",
			"device_product": "Firewall",
			"device_version": "1.0",
			"event_id":       "100",
			"name":           "Attack",
			"severity":       "10",
			"extension": map[string]interface{}{
				"src": "1.2.3.4",
				"dst": "5.6.7.8",
			},
		},
	}, got)
}

func TestRenderDebugJSON_LEEF(t *testing.T) {
	got := renderDebugJSON(t, SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "host",
		AppName:   "app",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("LEEF:1.0|Microsoft|MSExchange|2013 SP1|15345|src=1.2.3.4"),
	})

	// LEEF: name and severity omitted from the SIEM block.
	assert.Equal(t, map[string]interface{}{
		"message": "LEEF:1.0|Microsoft|MSExchange|2013 SP1|15345|src=1.2.3.4",
		"syslog": map[string]interface{}{
			"timestamp": "2003-10-11T22:14:15.003Z",
			"hostname":  "host",
			"appname":   "app",
			"procid":    "-",
			"msgid":     "-",
			"severity":  float64(6),
			"facility":  float64(1),
			"version":   "1",
		},
		"siem": map[string]interface{}{
			"format":         "LEEF",
			"version":        "1.0",
			"device_vendor":  "Microsoft",
			"device_product": "MSExchange",
			"device_version": "2013 SP1",
			"event_id":       "15345",
			"extension": map[string]interface{}{
				"src": "1.2.3.4",
			},
		},
	}, got)
}

// ---------------------------------------------------------------------------
// Optional field omission
// ---------------------------------------------------------------------------

func TestRender_OptionalFields_Omitted(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       -1,
		Timestamp: "Oct 11 22:14:15",
		Hostname:  "h",
		AppName:   "a",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("msg"),
	}

	sc := NewSyslogStructuredContent(parsed)
	sc.debugRender = true
	got, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(got, &data))

	syslogMap := data["syslog"].(map[string]interface{})

	_, hasSev := syslogMap["severity"]
	assert.False(t, hasSev, "severity should be omitted when Pri < 0")
	_, hasFac := syslogMap["facility"]
	assert.False(t, hasFac, "facility should be omitted when Pri < 0")
	_, hasVer := syslogMap["version"]
	assert.False(t, hasVer, "version should be omitted for BSD")
	_, hasSD := syslogMap["structured_data"]
	assert.False(t, hasSD, "structured_data should be omitted when nil")
	_, hasSIEM := data["siem"]
	assert.False(t, hasSIEM, "siem should be omitted when nil")
}

func TestRender_OptionalFields_Present(t *testing.T) {
	parsed := SyslogMessage{
		Pri:     165,
		Version: "1",
		StructuredData: map[string]map[string]string{
			"meta@1234": {"key": "val"},
		},
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "h",
		AppName:   "a",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("msg"),
	}

	sc := NewSyslogStructuredContent(parsed)
	sc.debugRender = true
	got, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(got, &data))

	syslogMap := data["syslog"].(map[string]interface{})
	assert.Equal(t, float64(5), syslogMap["severity"])
	assert.Equal(t, float64(20), syslogMap["facility"])
	assert.Equal(t, "1", syslogMap["version"])
	assert.NotNil(t, syslogMap["structured_data"])
}

// ---------------------------------------------------------------------------
// GetContent / SetContent
// ---------------------------------------------------------------------------

func TestGetContent(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri: 14, Timestamp: "-", Hostname: "-", AppName: "-",
		ProcID: "-", MsgID: "-", Msg: []byte("hello world"),
	})
	assert.Equal(t, []byte("hello world"), sc.GetContent())
}

func TestSetContent_RoundTrip(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri: 14, Timestamp: "-", Hostname: "-", AppName: "-",
		ProcID: "-", MsgID: "-", Msg: []byte("original"),
	})
	sc.debugRender = true

	sc.SetContent([]byte("scrubbed"))
	assert.Equal(t, []byte("scrubbed"), sc.GetContent())

	rendered, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))
	assert.Equal(t, "scrubbed", data["message"])
}

// ---------------------------------------------------------------------------
// GetAttribute
// ---------------------------------------------------------------------------

func TestGetAttribute_SyslogFields(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri:       165,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "myhost",
		AppName:   "myapp",
		ProcID:    "1234",
		MsgID:     "REQ1",
		StructuredData: map[string]map[string]string{
			"meta@1234": {"key": "val"},
		},
		Msg: []byte("hello"),
	})

	tests := []struct {
		path  string
		want  string
		found bool
	}{
		{"message", "hello", true},
		{"syslog.hostname", "myhost", true},
		{"syslog.appname", "myapp", true},
		{"syslog.procid", "1234", true},
		{"syslog.msgid", "REQ1", true},
		{"syslog.timestamp", "2003-10-11T22:14:15.003Z", true},
		{"syslog.severity", "5", true},
		{"syslog.facility", "20", true},
		{"syslog.version", "1", true},
		{"syslog.structured_data.meta@1234.key", "val", true},
		{"syslog.structured_data.nonexist.key", "", false},
		{"syslog.nonexist", "", false},
		{"nonexist", "", false},
		{"syslog", "", false},
		{"message.sub", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			val, ok := sc.GetAttribute(tc.path)
			assert.Equal(t, tc.found, ok)
			assert.Equal(t, tc.want, val)
		})
	}
}

func TestGetAttribute_SIEMFields(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "-",
		Hostname:  "-",
		AppName:   "-",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte(`CEF:0|Security|Firewall|1.0|100|Attack|10|src=1.2.3.4`),
	}
	sc := NewSyslogStructuredContent(parsed)

	tests := []struct {
		path  string
		want  string
		found bool
	}{
		{"siem.format", "CEF", true},
		{"siem.version", "0", true},
		{"siem.device_vendor", "Security", true},
		{"siem.device_product", "Firewall", true},
		{"siem.device_version", "1.0", true},
		{"siem.event_id", "100", true},
		{"siem.name", "Attack", true},
		{"siem.severity", "10", true},
		{"siem.extension.src", "1.2.3.4", true},
		{"siem.extension.nonexist", "", false},
		{"siem.nonexist", "", false},
		{"siem", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			val, ok := sc.GetAttribute(tc.path)
			assert.Equal(t, tc.found, ok)
			assert.Equal(t, tc.want, val)
		})
	}
}

func TestGetAttribute_NoPri_OmitsSeverityFacility(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri: -1, Timestamp: "-", Hostname: "-", AppName: "-",
		ProcID: "-", MsgID: "-", Msg: []byte("test"),
	})

	_, ok := sc.GetAttribute("syslog.severity")
	assert.False(t, ok)
	_, ok = sc.GetAttribute("syslog.facility")
	assert.False(t, ok)
}

func TestGetAttribute_NoSIEM(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri: 14, Timestamp: "-", Hostname: "-", AppName: "-",
		ProcID: "-", MsgID: "-", Msg: []byte("not CEF"),
	})

	_, ok := sc.GetAttribute("siem.format")
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// GetStructuredAttribute integration (via Message)
// ---------------------------------------------------------------------------

func TestGetStructuredAttribute_UsesSyslogStructuredContent(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri:       165,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "myhost",
		AppName:   "myapp",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("hello"),
	})

	msg := message.NewStructuredMessage(sc, nil, "info", 0)

	val, ok := msg.GetStructuredAttribute("syslog.hostname")
	assert.True(t, ok)
	assert.Equal(t, "myhost", val)

	val, ok = msg.GetStructuredAttribute("syslog.severity")
	assert.True(t, ok)
	assert.Equal(t, "5", val)

	val, ok = msg.GetStructuredAttribute("message")
	assert.True(t, ok)
	assert.Equal(t, "hello", val)
}

func TestSplitFirst_EscapedDots(t *testing.T) {
	tests := []struct {
		input  string
		first  string
		rest   string
		hasDot bool
	}{
		{"simple.path", "simple", "path", true},
		{`escaped\.dot.rest`, "escaped.dot", "rest", true},
		{`no_dots`, "no_dots", "", false},
		{`a\\.b`, `a\`, "b", true},
		{`a\.b\.c.d`, "a.b.c", "d", true},
		{`trailing\`, `trailing\`, "", false},
		{`\.`, ".", "", false},
		{`\\`, `\`, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			first, rest, hasDot := splitFirst(tc.input)
			assert.Equal(t, tc.first, first)
			assert.Equal(t, tc.rest, rest)
			assert.Equal(t, tc.hasDot, hasDot)
		})
	}
}

func TestUnescapeSegment(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`plain`, "plain"},
		{`dotted\.key`, "dotted.key"},
		{`back\\slash`, `back\slash`},
		{`no_escapes`, "no_escapes"},
		{`trailing\`, `trailing\`},
		{`a\.b\.c`, "a.b.c"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, unescapeSegment(tc.input))
		})
	}
}

func TestGetAttribute_EscapedDotSDID(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri:       165,
		Version:   "1",
		Timestamp: "-",
		Hostname:  "-",
		AppName:   "-",
		ProcID:    "-",
		MsgID:     "-",
		StructuredData: map[string]map[string]string{
			"my.org@99999": {"status": "ok"},
		},
		Msg: []byte("test"),
	})

	t.Run("escaped dot reaches dotted SD-ID", func(t *testing.T) {
		val, ok := sc.GetAttribute(`syslog.structured_data.my\.org@99999.status`)
		assert.True(t, ok)
		assert.Equal(t, "ok", val)
	})

	t.Run("unescaped dot in dotted SD-ID fails", func(t *testing.T) {
		_, ok := sc.GetAttribute("syslog.structured_data.my.org@99999.status")
		assert.False(t, ok)
	})

	t.Run("escaped dot in param name", func(t *testing.T) {
		sc2 := NewSyslogStructuredContent(SyslogMessage{
			Pri:       165,
			Version:   "1",
			Timestamp: "-",
			Hostname:  "-",
			AppName:   "-",
			ProcID:    "-",
			MsgID:     "-",
			StructuredData: map[string]map[string]string{
				"myid": {"param.name": "value"},
			},
			Msg: []byte("test"),
		})
		val, ok := sc2.GetAttribute(`syslog.structured_data.myid.param\.name`)
		assert.True(t, ok)
		assert.Equal(t, "value", val)
	})
}

func TestGetAttribute_EscapedDotSIEMExtension(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "-",
		Hostname:  "-",
		AppName:   "-",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte(`CEF:0|Security|Firewall|1.0|100|Attack|10|src=1.2.3.4`),
	}
	sc := NewSyslogStructuredContent(parsed)

	t.Run("plain extension key works", func(t *testing.T) {
		val, ok := sc.GetAttribute("siem.extension.src")
		assert.True(t, ok)
		assert.Equal(t, "1.2.3.4", val)
	})

	t.Run("escaped dot in extension key", func(t *testing.T) {
		sc2 := &SyslogStructuredContent{
			siem: &SIEMFields{
				Format:  "CEF",
				Version: "0",
				Extension: map[string]string{
					"dotted.key": "found",
				},
			},
		}
		val, ok := sc2.GetAttribute(`siem.extension.dotted\.key`)
		assert.True(t, ok)
		assert.Equal(t, "found", val)
	})

	t.Run("unescaped dot in extension key also works", func(t *testing.T) {
		// Extension keys are terminal — the entire rest after "extension."
		// is the map key, so dots are always literal. Escaping is optional.
		sc2 := &SyslogStructuredContent{
			siem: &SIEMFields{
				Format:  "CEF",
				Version: "0",
				Extension: map[string]string{
					"dotted.key": "found",
				},
			},
		}
		val, ok := sc2.GetAttribute("siem.extension.dotted.key")
		assert.True(t, ok)
		assert.Equal(t, "found", val)
	})
}

// ---------------------------------------------------------------------------
// debugRender flag: Render() behavior
// ---------------------------------------------------------------------------

func TestRender_ExtractAttrsDisabled_ReturnsRawMessage(t *testing.T) {
	rawInput := "<134>1 2024-01-01T00:00:00Z myhost myapp 1234 - - hello world"
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri: 134, Version: "1", Timestamp: "2024-01-01T00:00:00Z",
		Hostname: "myhost", AppName: "myapp", ProcID: "1234",
		MsgID: "-", Msg: []byte("hello world"),
	})
	sc.msg = rawInput
	sc.debugRender = false

	rendered, err := sc.Render()
	require.NoError(t, err)
	assert.Equal(t, rawInput, string(rendered))

	var data map[string]interface{}
	assert.Error(t, json.Unmarshal(rendered, &data), "should not be JSON")
}

func TestRender_ExtractAttrsEnabled_ReturnsJSON(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri: 134, Version: "1", Timestamp: "2024-01-01T00:00:00Z",
		Hostname: "myhost", AppName: "myapp", ProcID: "1234",
		MsgID: "-", Msg: []byte("hello world"),
	})
	sc.debugRender = true

	rendered, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))
	assert.Equal(t, "hello world", data["message"])

	syslog, ok := data["syslog"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "myhost", syslog["hostname"])
	assert.Equal(t, "myapp", syslog["appname"])
}

func TestRender_ExtractAttrsDisabled_GetAttributeStillWorks(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri: 134, Version: "1", Timestamp: "2024-01-01T00:00:00Z",
		Hostname: "myhost", AppName: "myapp", ProcID: "1234",
		MsgID: "-", Msg: []byte("hello world"),
	})
	sc.msg = "<134>1 2024-01-01T00:00:00Z myhost myapp 1234 - - hello world"
	sc.debugRender = false

	val, found := sc.GetAttribute("syslog.hostname")
	assert.True(t, found)
	assert.Equal(t, "myhost", val)

	val, found = sc.GetAttribute("syslog.appname")
	assert.True(t, found)
	assert.Equal(t, "myapp", val)
}

func TestRender_ExtractAttrsDisabled_CEF_PreservesOriginal(t *testing.T) {
	sc := NewSyslogStructuredContent(SyslogMessage{
		Pri: 134, Version: "1", Timestamp: "2024-01-01T00:00:00Z",
		Hostname: "host", AppName: "CEF", ProcID: "-", MsgID: "-",
		Msg: []byte("CEF:0|Security|IDS|1.0|100|Intrusion|9|src=10.0.0.1"),
	})
	sc.msg = "full original line"
	sc.debugRender = false

	rendered, err := sc.Render()
	require.NoError(t, err)
	assert.Equal(t, "full original line", string(rendered))

	// SIEM fields are still accessible internally
	assert.NotNil(t, sc.siem)
	val, found := sc.GetAttribute("siem.device_vendor")
	assert.True(t, found)
	assert.Equal(t, "Security", val)
}

// ---------------------------------------------------------------------------
// Render benchmarks (debug JSON path)
// ---------------------------------------------------------------------------

func BenchmarkRender(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", []byte(`<14>1 2003-10-11T22:14:15.003Z host app - - - short`)},
		{"RFC5424_Typical", []byte(`<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] An application event log entry`)},
		{"RFC5424_Long_1KB", []byte(`<14>1 2003-10-11T22:14:15.003Z longhost.example.com myservice 12345 REQ-001 [meta@1234 key="val"] ` + strings.Repeat("x", 1024))},
		{"BSD", []byte(`<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8`)},
	} {
		parsed, _ := Parse(tc.msg)
		sc := NewSyslogStructuredContent(parsed)
		sc.debugRender = true
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.msg)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := sc.Render(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestGetAttribute_EmptyFieldsNotFound(t *testing.T) {
	// Absent syslog string fields must be reported as absent, not
	// present-but-empty, so remap_source rules never match on a hollow value.
	// Parsers encode absence two ways: the empty string, and the nilvalue
	// sentinel "-" (RFC 5424 NILVALUE / parseBSDNoTimestamp placeholder). Both
	// must be treated as not-found.
	stringPaths := []string{
		"syslog.timestamp",
		"syslog.hostname",
		"syslog.appname",
		"syslog.procid",
		"syslog.msgid",
	}
	for _, sentinel := range []string{"", nilvalue} {
		sc := &SyslogStructuredContent{
			syslog: SyslogFields{
				Timestamp: sentinel,
				Hostname:  sentinel,
				AppName:   sentinel,
				ProcID:    sentinel,
				MsgID:     sentinel,
				Severity:  -1,
				Facility:  -1,
			},
		}
		for _, path := range stringPaths {
			val, ok := sc.GetAttribute(path)
			assert.False(t, ok, "%s should be absent for sentinel %q", path, sentinel)
			assert.Equal(t, "", val, "%s should return empty string for sentinel %q", path, sentinel)
		}
		// version/severity/facility are absent regardless of the string sentinel.
		for _, path := range []string{"syslog.version", "syslog.severity", "syslog.facility"} {
			val, ok := sc.GetAttribute(path)
			assert.False(t, ok, "%s should be absent", path)
			assert.Equal(t, "", val)
		}
	}
}

func TestGetAttribute_NilvalueFieldsNotFoundThroughParser(t *testing.T) {
	// Through the real parser: a digit-prefixed no-timestamp BSD message
	// (RFC 3164 §4.3.2, e.g. PAN-OS/CSV) encodes every absent header field as
	// the nilvalue sentinel "-". GetAttribute must not surface those as
	// present-with-value-"-", otherwise a remap_source rule keyed on, say,
	// syslog.appname would match every such message.
	parsed, err := Parse([]byte(`<134>1,2026/06/23,00053,TRAFFIC,end,payload`))
	require.NoError(t, err)
	sc := NewSyslogStructuredContent(parsed)
	for _, path := range []string{
		"syslog.timestamp",
		"syslog.hostname",
		"syslog.appname",
		"syslog.procid",
		"syslog.msgid",
	} {
		val, ok := sc.GetAttribute(path)
		assert.False(t, ok, "%s should be absent when parser encodes it as nilvalue", path)
		assert.Equal(t, "", val, "%s should return empty string when absent", path)
	}
}
