// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// ---------------------------------------------------------------------------
// JSON parity: SyslogStructuredContent vs BasicStructuredContent
// ---------------------------------------------------------------------------

func jsonEqual(t *testing.T, a, b []byte) {
	t.Helper()
	var aObj, bObj interface{}
	require.NoError(t, json.Unmarshal(a, &aObj))
	require.NoError(t, json.Unmarshal(b, &bObj))
	assert.Equal(t, aObj, bObj)
}

func buildBasicSC(parsed SyslogMessage) *message.BasicStructuredContent {
	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": string(parsed.Msg),
			"syslog":  BuildSyslogFields(parsed),
		},
	}
	if header, ext, _, ok := ParseCEFLEEF(parsed.Msg); ok {
		sc.Data["siem"] = BuildSIEMFields(header, ext)
		sc.Data["message"] = ""
	}
	return sc
}

func TestRenderParity_RFC5424(t *testing.T) {
	parsed := SyslogMessage{
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
	}

	old, err := buildBasicSC(parsed).Render()
	require.NoError(t, err)

	newSC := NewSyslogStructuredContent(parsed)
	got, err := newSC.Render()
	require.NoError(t, err)

	jsonEqual(t, old, got)
}

func TestRenderParity_RFC5424_Short(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "host",
		AppName:   "app",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("short"),
	}

	old, err := buildBasicSC(parsed).Render()
	require.NoError(t, err)

	got, err := NewSyslogStructuredContent(parsed).Render()
	require.NoError(t, err)

	jsonEqual(t, old, got)
}

func TestRenderParity_BSD(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       34,
		Timestamp: "Oct 11 22:14:15",
		Hostname:  "mymachine",
		AppName:   "su",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("'su root' failed for lonvick on /dev/pts/8"),
	}

	old, err := buildBasicSC(parsed).Render()
	require.NoError(t, err)

	got, err := NewSyslogStructuredContent(parsed).Render()
	require.NoError(t, err)

	jsonEqual(t, old, got)
}

func TestRenderParity_NoPri(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       -1,
		Timestamp: "Oct 11 22:14:15",
		Hostname:  "mymachine",
		AppName:   "syslogd",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("restart"),
	}

	old, err := buildBasicSC(parsed).Render()
	require.NoError(t, err)

	got, err := NewSyslogStructuredContent(parsed).Render()
	require.NoError(t, err)

	jsonEqual(t, old, got)
}

func TestRenderParity_NILVALUE(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "-",
		Hostname:  "-",
		AppName:   "-",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("test"),
	}

	old, err := buildBasicSC(parsed).Render()
	require.NoError(t, err)

	got, err := NewSyslogStructuredContent(parsed).Render()
	require.NoError(t, err)

	jsonEqual(t, old, got)
}

func TestRenderParity_CEF(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "host",
		AppName:   "app",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte(`CEF:0|Security|Firewall|1.0|100|Attack|10|src=1.2.3.4 dst=5.6.7.8`),
	}

	old, err := buildBasicSC(parsed).Render()
	require.NoError(t, err)

	got, err := NewSyslogStructuredContent(parsed).Render()
	require.NoError(t, err)

	jsonEqual(t, old, got)
}

func TestRenderParity_LEEF(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "host",
		AppName:   "app",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("LEEF:1.0|Microsoft|MSExchange|2013 SP1|15345|src=1.2.3.4"),
	}

	old, err := buildBasicSC(parsed).Render()
	require.NoError(t, err)

	got, err := NewSyslogStructuredContent(parsed).Render()
	require.NoError(t, err)

	jsonEqual(t, old, got)
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

	got, err := NewSyslogStructuredContent(parsed).Render()
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

	got, err := NewSyslogStructuredContent(parsed).Render()
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
// Interface compliance
// ---------------------------------------------------------------------------

func TestSyslogStructuredContent_ImplementsStructuredContent(t *testing.T) {
	var _ message.StructuredContent = (*SyslogStructuredContent)(nil)
}

func TestSyslogStructuredContent_ImplementsAttributeGetter(t *testing.T) {
	var _ message.AttributeGetter = (*SyslogStructuredContent)(nil)
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
