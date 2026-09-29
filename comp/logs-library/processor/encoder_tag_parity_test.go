// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// encoderTagParityCase pins the full encoded bytes each encoder emits for one
// tailer family's tag wiring. Expected values are what main emits today.
type encoderTagParityCase struct {
	name     string
	attached []string // what the tailer passes to Origin.SetTags

	wantRaw   string
	wantJSON  string
	wantProto []string
}

var encoderParityTimestamp = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

const encoderParityHostname = "test-host"

func newEncoderParityConfig() *config.LogsConfig {
	return &config.LogsConfig{
		Service:        "web-svc",
		Source:         "nginx",
		SourceCategory: "web",
		Tags:           []string{"env:prod", "team:infra"},
	}
}

var encoderTagParityCases = []encoderTagParityCase{
	{
		name:      "file",
		attached:  []string{"filename:app.log", "dirname:/var/log", "container_name:nginx", "truncated:single_line"},
		wantRaw:   `<46>0 2026-09-29T12:00:00.000000000Z test-host web-svc - - [dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,filename:app.log,dirname:/var/log,container_name:nginx,truncated:single_line"] hello`,
		wantJSON:  `{"message":"hello","status":"info","timestamp":1790683200000,"hostname":"test-host","service":"web-svc","ddsource":"nginx","ddtags":"filename:app.log,dirname:/var/log,container_name:nginx,truncated:single_line,sourcecategory:web,env:prod,team:infra"}`,
		wantProto: []string{"filename:app.log", "dirname:/var/log", "container_name:nginx", "truncated:single_line", "sourcecategory:web", "env:prod", "team:infra"},
	},
	{
		name:      "container",
		attached:  []string{"truncated:single_line", "noisy_log:true", "container_name:nginx", "image_name:nginx"},
		wantRaw:   `<46>0 2026-09-29T12:00:00.000000000Z test-host web-svc - - [dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:single_line,noisy_log:true,container_name:nginx,image_name:nginx"] hello`,
		wantJSON:  `{"message":"hello","status":"info","timestamp":1790683200000,"hostname":"test-host","service":"web-svc","ddsource":"nginx","ddtags":"truncated:single_line,noisy_log:true,container_name:nginx,image_name:nginx,sourcecategory:web,env:prod,team:infra"}`,
		wantProto: []string{"truncated:single_line", "noisy_log:true", "container_name:nginx", "image_name:nginx", "sourcecategory:web", "env:prod", "team:infra"},
	},
	{
		name:      "socket",
		attached:  []string{"truncated:single_line"},
		wantRaw:   `<46>0 2026-09-29T12:00:00.000000000Z test-host web-svc - - [dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:single_line"] hello`,
		wantJSON:  `{"message":"hello","status":"info","timestamp":1790683200000,"hostname":"test-host","service":"web-svc","ddsource":"nginx","ddtags":"truncated:single_line,sourcecategory:web,env:prod,team:infra"}`,
		wantProto: []string{"truncated:single_line", "sourcecategory:web", "env:prod", "team:infra"},
	},
	{
		// journald after PR0 (#55908): configured tags are not attached.
		name:      "journald",
		attached:  []string{"container_name:nginx"},
		wantRaw:   `<46>0 2026-09-29T12:00:00.000000000Z test-host web-svc - - [dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,container_name:nginx"] hello`,
		wantJSON:  `{"message":"hello","status":"info","timestamp":1790683200000,"hostname":"test-host","service":"web-svc","ddsource":"nginx","ddtags":"container_name:nginx,sourcecategory:web,env:prod,team:infra"}`,
		wantProto: []string{"container_name:nginx", "sourcecategory:web", "env:prod", "team:infra"},
	},
	{
		// windowsevent after PR0 (#55908): only parsing tags are attached.
		name:      "windowsevent",
		attached:  []string{"truncated:true"},
		wantRaw:   `<46>0 2026-09-29T12:00:00.000000000Z test-host web-svc - - [dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:true"] hello`,
		wantJSON:  `{"message":"hello","status":"info","timestamp":1790683200000,"hostname":"test-host","service":"web-svc","ddsource":"nginx","ddtags":"truncated:true,sourcecategory:web,env:prod,team:infra"}`,
		wantProto: []string{"truncated:true", "sourcecategory:web", "env:prod", "team:infra"},
	},
	{
		name:      "no attached tags",
		attached:  nil,
		wantRaw:   `<46>0 2026-09-29T12:00:00.000000000Z test-host web-svc - - [dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra"] hello`,
		wantJSON:  `{"message":"hello","status":"info","timestamp":1790683200000,"hostname":"test-host","service":"web-svc","ddsource":"nginx","ddtags":"sourcecategory:web,env:prod,team:infra"}`,
		wantProto: []string{"sourcecategory:web", "env:prod", "team:infra"},
	},
}

// newEncoderParityMessage builds a rendered message whose Origin is wired the
// way a tailer wires it.
func newEncoderParityMessage(cfg *config.LogsConfig, attached []string) *message.Message {
	source := sources.NewLogSource("", cfg)
	origin := message.NewOrigin(source)
	origin.SetTags(attached)
	msg := message.NewMessage([]byte("hello"), origin, message.StatusInfo, 0)
	msg.State = message.StateRendered
	msg.ServerlessExtra.Timestamp = encoderParityTimestamp
	return msg
}

// encodeForParity runs one encoder on a fresh message and returns its bytes.
func encodeForParity(t *testing.T, encoder Encoder, msg *message.Message) []byte {
	t.Helper()
	require.NoError(t, encoder.Encode(msg, encoderParityHostname))
	return msg.GetContent()
}

// assertEncoderTagParity checks raw, JSON and protobuf output for one case.
// newMsg must return a fresh message for every call: encoding mutates it.
func assertEncoderTagParity(t *testing.T, c encoderTagParityCase, newMsg func() *message.Message) {
	t.Helper()

	assert.Equal(t, c.wantRaw, string(encodeForParity(t, NewRawEncoder(false), newMsg())), "raw")
	assert.Equal(t, c.wantJSON, string(encodeForParity(t, JSONEncoder, newMsg())), "json")

	log := &pb.Log{}
	require.NoError(t, log.Unmarshal(encodeForParity(t, ProtoEncoder, newMsg())))
	assert.Equal(t, c.wantProto, log.Tags, "protobuf tags")
	assert.Equal(t, "nginx", log.Source, "protobuf source")
	assert.Equal(t, "web-svc", log.Service, "protobuf service")
	assert.Equal(t, encoderParityHostname, log.Hostname, "protobuf hostname")
	assert.Equal(t, encoderParityTimestamp.UnixNano(), log.Timestamp, "protobuf timestamp")
}

// TestEncoderTagWireParity pins the raw, JSON and protobuf bytes for each
// tailer family's tag wiring.
func TestEncoderTagWireParity(t *testing.T) {
	for _, c := range encoderTagParityCases {
		t.Run(c.name, func(t *testing.T) {
			assertEncoderTagParity(t, c, func() *message.Message {
				return newEncoderParityMessage(newEncoderParityConfig(), c.attached)
			})
		})
	}
}
