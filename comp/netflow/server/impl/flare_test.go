// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package serverimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/decoders/netflow/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	forwardermock "github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/mock"
	server "github.com/DataDog/datadog-agent/comp/netflow/server/def"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestFillFlare_NetFlow9 starts a real NetFlow v9 listener, sends a packet
// containing template + data records, waits for the agent to decode and flush
// the flows (which proves templates were cached), and then exercises
// fillFlare end-to-end. It validates the human-readable, JSON, and pcap
// outputs — including round-tripping the pcap through gopacket to confirm the
// synthetic Ethernet/IPv4/UDP framing matches the recorded exporter.
func TestFillFlare_NetFlow9(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)

	var epForwarder forwardermock.MockComponent
	srv := fxutil.Test[server.Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(singleListenerConfig("netflow9", port)),
		setTimeNow,
	)).(*Server)

	// We don't care about exact event counts here — only that decoding
	// happened (which is what populates the template cache).
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkDevicesNetFlow).Return(nil).AnyTimes()
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).AnyTimes()

	packet, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting netflow9 packet")

	// Sending the packet and waiting for flushed flows guarantees the
	// listener fully decoded the data records, which in turn requires that
	// templates were cached first.
	require.True(t, assertFlowEventsCount(t, port, srv, packet, 29))

	fb := helpers.NewFlareBuilderMock(t, false)
	require.NoError(t, srv.fillFlare(context.Background(), fb.FlareBuilder))

	// Text + JSON exports.
	fb.AssertFileExists("netflow", "templates_readable.txt")
	fb.AssertFileExists("netflow", "templates.json")
	fb.AssertFileContentMatch("(?s)NetFlow Template Export", "netflow", "templates_readable.txt")

	rawJSON, err := os.ReadFile(filepath.Join(fb.Root, "netflow", "templates.json"))
	require.NoError(t, err)
	var entries []map[string]any
	require.NoError(t, json.Unmarshal(rawJSON, &entries))
	require.NotEmpty(t, entries, "expected at least one exported template")
	foundLoopbackV9 := false
	for _, e := range entries {
		if e["exporter_ip"] == "127.0.0.1" {
			assert.EqualValues(t, 9, e["version"])
			foundLoopbackV9 = true
		}
	}
	require.True(t, foundLoopbackV9, "expected templates from 127.0.0.1 in JSON, got: %v", entries)

	// Locate the pcap regardless of observation domain id.
	pcapMatches, err := filepath.Glob(filepath.Join(fb.Root, "netflow", "templates_127.0.0.1_v9_obs*.pcap"))
	require.NoError(t, err)
	require.NotEmpty(t, pcapMatches, "expected a templates pcap for 127.0.0.1")

	pcapPath := pcapMatches[0]
	pf, err := os.Open(pcapPath)
	require.NoError(t, err)
	defer pf.Close()

	r, err := pcapgo.NewReader(pf)
	require.NoError(t, err)
	require.EqualValues(t, layers.LinkTypeEthernet, r.LinkType())

	data, _, err := r.ReadPacketData()
	require.NoError(t, err, "pcap should contain at least one frame")

	pkt := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
	if errLayer := pkt.ErrorLayer(); errLayer != nil {
		t.Fatalf("gopacket failed to dissect synthetic frame: %v", errLayer.Error())
	}
	require.NotNil(t, pkt.Layer(layers.LayerTypeEthernet), "expected Ethernet layer")

	ipLayer := pkt.Layer(layers.LayerTypeIPv4)
	require.NotNil(t, ipLayer, "expected IPv4 layer")
	ip := ipLayer.(*layers.IPv4)
	require.Equal(t, "127.0.0.1", ip.SrcIP.String(), "src IP must match recorded exporter")
	require.EqualValues(t, ipProtoUDP, ip.Protocol)

	udpLayer := pkt.Layer(layers.LayerTypeUDP)
	require.NotNil(t, udpLayer, "expected UDP layer")
	udp := udpLayer.(*layers.UDP)
	require.EqualValues(t, netflowV9DstPort, udp.DstPort, "dst port should be NetFlow v9 default")

	// Payload begins with NetFlow v9 version field (big-endian uint16 = 9).
	require.GreaterOrEqual(t, len(udp.Payload), 2, "UDP payload too short")
	require.Equal(t, byte(0x00), udp.Payload[0])
	require.Equal(t, byte(0x09), udp.Payload[1])

	// README accompanies the pcap.
	readme := strings.TrimSuffix(filepath.Base(pcapPath), ".pcap") + "_README.txt"
	fb.AssertFileExists("netflow", readme)
	fb.AssertFileContentMatch("mergecap -a", "netflow", readme)

	// Round-trip the synthesized UDP payload through goflow2's NetFlow
	// decoder against a fresh in-memory template cache. This guards against
	// regressions in the NetFlow v9 / IPFIX encoding helpers — any malformed
	// header, flowset, or template record would surface here, even if the
	// outer pcap framing is fine.
	ctx := context.Background()
	freshTS, err := templates.FindTemplateSystem(ctx, "memory")
	require.NoError(t, err)
	defer freshTS.Close(ctx)

	decoderKey := ip.SrcIP.String()
	_, err = netflow.DecodeMessageContext(ctx, bytes.NewBuffer(udp.Payload), decoderKey,
		netflow.TemplateWrapper{Ctx: ctx, Key: decoderKey, Inner: freshTS})
	require.NoError(t, err, "synthetic NetFlow payload failed to decode")

	roundTripChan := make(chan *templates.TemplateKey, 64)
	go func() {
		defer close(roundTripChan)
		require.NoError(t, freshTS.ListTemplates(ctx, roundTripChan))
	}()
	var roundTripCount int
	for k := range roundTripChan {
		if k == nil {
			break
		}
		roundTripCount++
	}
	require.Greater(t, roundTripCount, 0, "expected at least one template to be re-decodable from the synthesized pcap")
}
