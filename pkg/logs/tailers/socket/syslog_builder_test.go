// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	syslogparser "github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/syslog"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestStructuredContent_Render_RFC5424(t *testing.T) {
	parsed := syslogparser.SyslogMessage{
		Pri:       165,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "mymachine",
		AppName:   "evntslog",
		ProcID:    "-",
		MsgID:     "ID47",
		StructuredData: map[string]map[string]string{
			"exampleSDID@32473": {"iut": "3"},
		},
		Msg: []byte("An application event log entry"),
	}

	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": string(parsed.Msg),
			"syslog":  syslogparser.BuildSyslogFields(parsed),
		},
	}

	rendered, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(rendered, &data)
	require.NoError(t, err)

	assert.Equal(t, "An application event log entry", data["message"])

	syslogMap, ok := data["syslog"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, float64(5), syslogMap["severity"])
	assert.Equal(t, float64(20), syslogMap["facility"])
	assert.Equal(t, "1", syslogMap["version"])
	assert.Equal(t, "2003-10-11T22:14:15.003Z", syslogMap["timestamp"])
	assert.Equal(t, "mymachine", syslogMap["hostname"])
	assert.Equal(t, "evntslog", syslogMap["appname"])
	assert.Equal(t, "-", syslogMap["procid"])
	assert.Equal(t, "ID47", syslogMap["msgid"])

	sdRaw, ok := syslogMap["structured_data"].(map[string]interface{})
	require.True(t, ok, "structured_data should be a map")
	elemRaw, ok := sdRaw["exampleSDID@32473"].(map[string]interface{})
	require.True(t, ok, "SD element should be a map")
	assert.Equal(t, "3", elemRaw["iut"])
}

func TestStructuredContent_Render_BSD(t *testing.T) {
	parsed := syslogparser.SyslogMessage{
		Pri:       38,
		Timestamp: "Oct 11 22:14:15",
		Hostname:  "mymachine",
		AppName:   "su",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("'su root' failed for lonvick on /dev/pts/8"),
	}

	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": string(parsed.Msg),
			"syslog":  syslogparser.BuildSyslogFields(parsed),
		},
	}

	rendered, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(rendered, &data)
	require.NoError(t, err)

	syslogMap := data["syslog"].(map[string]interface{})

	_, hasVersion := syslogMap["version"]
	assert.False(t, hasVersion, "BSD messages should not have version")
	_, hasSD := syslogMap["structured_data"]
	assert.False(t, hasSD, "BSD messages should not have structured_data")

	assert.Equal(t, float64(6), syslogMap["severity"])
	assert.Equal(t, float64(4), syslogMap["facility"])
}

func TestStructuredContent_Render_NoPri(t *testing.T) {
	parsed := syslogparser.SyslogMessage{
		Pri:       -1,
		Timestamp: "Oct 11 22:14:15",
		Hostname:  "mymachine",
		AppName:   "syslogd",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("restart"),
	}

	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": string(parsed.Msg),
			"syslog":  syslogparser.BuildSyslogFields(parsed),
		},
	}

	rendered, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(rendered, &data)
	require.NoError(t, err)

	syslogMap := data["syslog"].(map[string]interface{})
	_, hasSev := syslogMap["severity"]
	assert.False(t, hasSev, "Pri=-1 should omit severity")
	_, hasFac := syslogMap["facility"]
	assert.False(t, hasFac, "Pri=-1 should omit facility")
}

func TestStructuredContent_GetContent(t *testing.T) {
	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": "hello world",
		},
	}
	assert.Equal(t, []byte("hello world"), sc.GetContent())
}

func TestStructuredContent_SetContent(t *testing.T) {
	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": "original message",
		},
	}

	sc.SetContent([]byte("scrubbed message"))
	assert.Equal(t, []byte("scrubbed message"), sc.GetContent())

	rendered, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(rendered, &data)
	require.NoError(t, err)
	assert.Equal(t, "scrubbed message", data["message"])
}

func TestStructuredContent_NILVALUEPreserved(t *testing.T) {
	parsed := syslogparser.SyslogMessage{
		Pri:       14,
		Version:   "1",
		Timestamp: "-",
		Hostname:  "-",
		AppName:   "-",
		ProcID:    "-",
		MsgID:     "-",
		Msg:       []byte("test"),
	}

	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": string(parsed.Msg),
			"syslog":  syslogparser.BuildSyslogFields(parsed),
		},
	}

	rendered, err := sc.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(rendered, &data)
	require.NoError(t, err)

	syslogMap := data["syslog"].(map[string]interface{})
	assert.Equal(t, "-", syslogMap["timestamp"])
	assert.Equal(t, "-", syslogMap["hostname"])
	assert.Equal(t, "-", syslogMap["appname"])
	assert.Equal(t, "-", syslogMap["procid"])
	assert.Equal(t, "-", syslogMap["msgid"])
}
