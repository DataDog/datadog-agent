package encoding

import (
	"encoding/json"
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSerialization(t *testing.T) {
	var httpReqStats http.RequestStats
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
				LastTCPEstablished:   1,
				LastTCPClosed:        1,
				MonotonicRetransmits: 201,
				LastRetransmits:      201,
				Pid:                  6000,
				NetNS:                7,
				SPort:                1000,
				DPort:                9000,
				IPTranslation: &network.IPTranslation{
					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
					ReplDstIP:   util.AddressFromString("20.1.1.1"),
					ReplSrcPort: 40,
					ReplDstPort: 70,
				},

				Type:      network.UDP,
				Family:    network.AFINET6,
				Direction: network.LOCAL,

				DNSCountByRcode: map[uint32]uint32{0: 1},
				DNSStatsByDomain: map[string]network.DNSStats{
					"foo.com": {
						DNSTimeouts:          0,
						DNSSuccessLatencySum: 0,
						DNSFailureLatencySum: 0,
						DNSCountByRcode:      map[uint32]uint32{0: 1},
					},
				},

				HTTPStatsByPath: map[string]http.RequestStats{
					"/testpath": httpReqStats,
				},
			},
		},
		DNS: map[util.Address][]string{
			util.AddressFromString("172.217.12.145"): {"golang.org"},
		},
	}

	out := &model.Connections{
		Conns: []*model.Connection{
			{
				Laddr:              &model.Addr{Ip: "10.1.1.1", Port: int32(1000)},
				Raddr:              &model.Addr{Ip: "10.2.2.2", Port: int32(9000)},
				LastBytesSent:      2,
				LastBytesReceived:  101,
				LastRetransmits:    201,
				LastTcpEstablished: 1,
				LastTcpClosed:      1,
				Pid:                int32(6000),
				NetNS:              7,
				IpTranslation: &model.IPTranslation{
					ReplSrcIP:   "20.1.1.1",
					ReplDstIP:   "20.1.1.1",
					ReplSrcPort: int32(40),
					ReplDstPort: int32(70),
				},

				Type:      model.ConnectionType_udp,
				Family:    model.ConnectionFamily_v6,
				Direction: model.ConnectionDirection_local,

				DnsCountByRcode: map[uint32]uint32{0: 1},
				DnsStatsByDomain: map[int32]*model.DNSStats{
					0: {
						DnsTimeouts:          0,
						DnsSuccessLatencySum: 0,
						DnsFailureLatencySum: 0,
						DnsCountByRcode:      map[uint32]uint32{0: 1},
					},
				},

				HttpStatsByPath: map[string]*model.HTTPStats{
					"/testpath": {
						StatsByResponseStatus: []*model.HTTPStats_Data{
							{
								Count:     0,
								Latencies: nil,
							},
							{
								Count:     0,
								Latencies: nil,
							},
							{
								Count:     0,
								Latencies: nil,
							},
							{
								Count:     0,
								Latencies: nil,
							},
							{
								Count:     0,
								Latencies: nil,
							},
						},
					},
				},
			},
		},
		Dns: map[string]*model.DNSEntry{
			"172.217.12.145": {Names: []string{"golang.org"}},
		},
		Domains: []string{"foo.com"},
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

func TestFormatHTTPStatsByPath(t *testing.T) {
	var httpReqStats http.RequestStats
	httpReqStats.AddRequest(100, 12.5)
	httpReqStats.AddRequest(405, 3.5)

	// Verify the latency data is correct prior to serialization
	latencies := httpReqStats.Latencies(model.HTTPResponseStatus_Info)
	assert.Equal(t, 1.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 12.5)

	latencies = httpReqStats.Latencies(model.HTTPResponseStatus_ClientErr)
	assert.Equal(t, 1.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 3.5)

	statsByPath := map[string]http.RequestStats{
		"/testpath": httpReqStats,
	}
	formattedStats := formatHTTPStatsByPath(statsByPath)

	// Deserialize the encoded latency information & confirm it is correct
	statsByResponseStatus := formattedStats["/testpath"].StatsByResponseStatus
	assert.Len(t, statsByResponseStatus, 5)

	serializedLatencies := statsByResponseStatus[model.HTTPResponseStatus_Info].Latencies
	sketch := unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 1.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 12.5)

	serializedLatencies = statsByResponseStatus[model.HTTPResponseStatus_ClientErr].Latencies
	sketch = unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 1.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 3.5)

	serializedLatencies = statsByResponseStatus[model.HTTPResponseStatus_Success].Latencies
	assert.Nil(t, serializedLatencies)
}

func unmarshalSketch(t *testing.T, bytes []byte) *ddsketch.DDSketch {
	var sketchPb sketchpb.DDSketch
	err := proto.Unmarshal(bytes, &sketchPb)
	assert.Nil(t, err)

	var sketch *ddsketch.DDSketch
	ret, err := sketch.FromProto(&sketchPb)
	assert.Nil(t, err)

	return ret
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * http.RelativeAccuracy
	assert.True(t, val >= expectedValue-acceptableError)
	assert.True(t, val <= expectedValue+acceptableError)
}
