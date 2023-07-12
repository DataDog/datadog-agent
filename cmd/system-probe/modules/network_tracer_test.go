// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestDecode(t *testing.T) {
	rec := httptest.NewRecorder()

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source: util.AddressFromString("10.1.1.1"),
					Dest:   util.AddressFromString("10.2.2.2"),
					Monotonic: network.StatCounters{
						SentBytes:   1,
						RecvBytes:   100,
						Retransmits: 201,
					},
					Last: network.StatCounters{
						SentBytes:   2,
						RecvBytes:   101,
						Retransmits: 201,
					},
					LastUpdateEpoch: 50,
					Pid:             6000,
					NetNS:           7,
					SPort:           1000,
					DPort:           9000,
					IPTranslation: &network.IPTranslation{
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
