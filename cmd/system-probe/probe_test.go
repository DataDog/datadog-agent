package main

import (
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	rec := httptest.NewRecorder()

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
	}

	marshaller := encoding.GetMarshaler(encoding.ContentTypeJSON)
	expected, err := marshaller.Marshal(in)
	require.NoError(t, err)

	writeConnections(rec, marshaller, in)

	rec.Flush()
	out := rec.Body.Bytes()
	assert.Equal(t, expected, out)

}
