package encoding

import (
	"encoding/json"
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSerialization(t *testing.T) {
	in := &network.Connections{
		Conns: []network.ConnectionStats{
			{
				Source:               util.AddressFromString("10.1.1.1"),
				Dest:                 util.AddressFromString("10.2.2.2"),
				MonotonicSentBytes:   1,
				LastSentBytes:        2,
				MonotonicRecvBytes:   100,
				LastRecvBytes:        101,
				LastUpdateEpoch:      50,
				MonotonicRetransmits: 201,
				LastRetransmits:      201,
				Pid:                  6000,
				NetNS:                7,
				SPort:                1000,
				DPort:                9000,
				IPTranslation: &netlink.IPTranslation{
					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
					ReplDstIP:   util.AddressFromString("20.1.1.1"),
					ReplSrcPort: 40,
					ReplDstPort: 70,
				},

				Type:      network.UDP,
				Family:    network.AFINET6,
				Direction: network.LOCAL,
			},
		},
		DNS: map[util.Address][]string{
			util.AddressFromString("172.217.12.145"): {"golang.org"},
		},
	}

	out := &model.Connections{
		Conns: []*model.Connection{
			{
				Laddr:             &model.Addr{Ip: "10.1.1.1", Port: int32(1000)},
				Raddr:             &model.Addr{Ip: "10.2.2.2", Port: int32(9000)},
				LastBytesSent:     2,
				LastBytesReceived: 101,
				LastRetransmits:   201,
				Pid:               int32(6000),
				NetNS:             7,
				IpTranslation: &model.IPTranslation{
					ReplSrcIP:   "20.1.1.1",
					ReplDstIP:   "20.1.1.1",
					ReplSrcPort: int32(40),
					ReplDstPort: int32(70),
				},

				Type:      model.ConnectionType_udp,
				Family:    model.ConnectionFamily_v6,
				Direction: model.ConnectionDirection_local,
			},
		},
		Dns: map[string]*model.DNSEntry{
			"172.217.12.145": {Names: []string{"golang.org"}},
		},
	}

	t.Run("requesting application/json serialization", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/json")
		assert.Equal("application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.Equal(out, result)
	})

	t.Run("requesting empty serialization", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("")
		// in case we request empty serialization type, default to application/json
		assert.Equal("application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.Equal(out, result)
	})

	t.Run("requesting application/protobuf serialization", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/protobuf")
		assert.Equal("application/protobuf", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.Equal(out, result)
	})

	t.Run("requesting unsupported serialization format", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/whatever")

		// In case we request an unsupported serialization type, we default to application/json
		assert.Equal("application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.Equal(out, result)
	})

	t.Run("render default values with application/json", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/json")
		assert.Equal("application/json", marshaler.ContentType())

		// Empty connection batch
		blob, err := marshaler.Marshal(&network.Connections{Conns: []network.ConnectionStats{{}}})
		require.NoError(t, err)

		res := struct {
			Conns []map[string]interface{} `json:"conns"`
		}{}
		require.NoError(t, json.Unmarshal(blob, &res))

		require.Len(t, res.Conns, 1)
		// Check that it contains fields even if they are zeroed
		for _, field := range []string{
			"type", "lastBytesSent", "lastBytesReceived", "lastRetransmits",
			"netNS", "family", "direction", "pid",
		} {
			assert.Contains(res.Conns[0], field)
		}
	})
}
