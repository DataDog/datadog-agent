// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	//	"sort"
	"testing"

	krpretty "github.com/kr/pretty"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/connections_bytes
var connectionsData []byte

func TestConnections(t *testing.T) {
	t.Run("parseConnectionsPayload should return error on invalid data", func(t *testing.T) {
		checks, err := ParseConnections(api.Payload{Data: []byte(""), Encoding: encodingProtobuf})
		assert.Error(t, err)
		assert.Empty(t, checks)
	})

	t.Run("parseConnectionsPayload should return valid checks on valid ", func(t *testing.T) {
		cc, err := ParseConnections(api.Payload{Data: connectionsData, Encoding: encodingProtobuf})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cc))
		assert.Equal(t, 17, len(cc[0].Connections))
		c := cc[0].Connections[16]
		t.Cleanup(func() {
			if t.Failed() {
				t.Log(krpretty.Sprint(c))
			}
		})
		assert.Equal(t, int32(11461), c.Pid)
		assert.Equal(t, "127.0.0.1", c.Laddr.Ip)
		assert.Equal(t, int32(8125), c.Laddr.Port)

		assert.Equal(t, uint64(143), c.LastPacketsReceived)
		assert.Equal(t, uint32(0xf0000000), c.NetNS)
	})
}
