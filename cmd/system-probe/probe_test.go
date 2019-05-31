package main

import (
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	rec := httptest.NewRecorder()

	in := &ebpf.Connections{
		Conns: []ebpf.ConnectionStats{
			{
				Source: util.AddressFromString("10.1.1.1"),
				Dest:   util.AddressFromString("10.1.1.1"),
				SPort:  1000,
				DPort:  9000,
				IPTranslation: &netlink.IPTranslation{
					ReplSrcIP: "20.1.1.1",
					ReplDstIP: "20.1.1.1",
				},
			},
		},
	}

	expected, err := in.MarshalJSON()
	require.NoError(t, err)

	writeConnections(rec, in)

	rec.Flush()
	out := rec.Body.Bytes()
	assert.Equal(t, expected, out)

}
