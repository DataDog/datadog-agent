// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var darwinPacketSidecarTelemetry = struct {
	packets       telemetry.Counter
	decodeErrors  telemetry.Counter
	unmatched     telemetry.Counter
	ambiguous     telemetry.Counter
	truncated     telemetry.Counter
	runtimeErrors telemetry.Counter
}{
	packets:       telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_packet", "packets", nil, "Packets inspected by the Darwin NStat sidecar"),
	decodeErrors:  telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_packet", "decode_errors", nil, "Packets rejected by the Darwin NStat sidecar decoder"),
	unmatched:     telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_packet", "unmatched", nil, "Packets without an authoritative NStat tuple"),
	ambiguous:     telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_packet", "ambiguous", nil, "Packets rejected because multiple NStat tuples matched"),
	truncated:     telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_packet", "truncated", nil, "Packets truncated by capture snap length"),
	runtimeErrors: telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_packet", "runtime_errors", nil, "Packet sidecar runtime failures"),
}

var darwinPacketMatchRate = telemetryimpl.GetCompatComponent().NewGauge(
	"network_tracer__darwin_packet",
	"match_rate",
	nil,
	"Fraction of inspected TCP packets uniquely matched to an NStat source",
)

type darwinPacketSidecarStats struct {
	packets      int64
	unmatched    int64
	ambiguous    int64
	decodeErrors int64
	stopped      bool
}

type darwinPacketSidecar struct {
	source   filter.PacketSource
	primary  *nstatTracer
	analyzer *darwinPacketAnalyzer

	onFailure func(error)
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	statsMu sync.Mutex
	stats   darwinPacketSidecarStats
}

func newDarwinPacketSidecar(source filter.PacketSource, primary *nstatTracer, maxFlows int) *darwinPacketSidecar {
	return &darwinPacketSidecar{
		source:   source,
		primary:  primary,
		analyzer: newDarwinPacketAnalyzer(maxFlows),
	}
}

func (s *darwinPacketSidecar) start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.visitPackets(); err != nil {
				darwinPacketSidecarTelemetry.runtimeErrors.Inc()
				s.markStopped()
				log.Warnf("notable: darwin_packet_enrichment stopped: %v", err)
				if s.onFailure != nil {
					s.onFailure(err)
				}
			}
		}()
	})
}

func (s *darwinPacketSidecar) visitPackets() error {
	var ethernet layers.Ethernet
	var loopback layers.Loopback
	var ip4 layers.IPv4
	var ip6 layers.IPv6
	var tcp layers.TCP
	var udp layers.UDP
	decoded := make([]gopacket.LayerType, 0, 5)
	ethernetParser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &ethernet, &ip4, &ip6, &tcp, &udp)
	ethernetParser.IgnoreUnsupported = true
	loopbackParser := gopacket.NewDecodingLayerParser(layers.LayerTypeLoopback, &loopback, &ip4, &ip6, &tcp, &udp)
	loopbackParser.IgnoreUnsupported = true

	return s.source.VisitPackets(func(data []byte, info filter.PacketInfo, _ time.Time) error {
		darwinPacketSidecarTelemetry.packets.Inc()
		s.recordPacket()
		parser := ethernetParser
		if info.LinkLayerType() == layers.LayerTypeLoopback {
			parser = loopbackParser
		}
		if err := parser.DecodeLayers(data, &decoded); err != nil {
			darwinPacketSidecarTelemetry.decodeErrors.Inc()
			s.recordDecodeError()
			return nil
		}
		pktType := info.PacketType()
		if pktType != filter.PacketHost && pktType != filter.PacketOutgoing {
			return nil
		}
		tuple, flags := buildTuple(pktType, &ip4, &ip6, &udp, &tcp, decoded)
		if !flags.tcpPresent || (!flags.ip4Present && !flags.ip6Present) {
			return nil
		}
		captureTruncated := info.OriginalLength() > info.CapturedLength() && info.CapturedLength() > 0
		if captureTruncated {
			darwinPacketSidecarTelemetry.truncated.Inc()
		}
		match := s.primary.enrichTCPPacket(
			network.ConnectionTuple(ebpfless.MakeConnStatsTuple(tuple)),
			pktType == filter.PacketOutgoing,
			captureTruncated,
			&tcp,
			s.analyzer,
		)
		switch {
		case match.ambiguous:
			darwinPacketSidecarTelemetry.ambiguous.Inc()
			s.recordAmbiguous()
		case !match.matched:
			darwinPacketSidecarTelemetry.unmatched.Inc()
			s.recordUnmatched()
		}
		s.publishMatchRate()
		return nil
	})
}

func (s *darwinPacketSidecar) snapshot() darwinPacketSidecarStats {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	return s.stats
}

func (s *darwinPacketSidecar) markStopped() {
	s.statsMu.Lock()
	s.stats.stopped = true
	s.statsMu.Unlock()
}

func (s *darwinPacketSidecar) recordPacket() {
	s.statsMu.Lock()
	s.stats.packets++
	s.statsMu.Unlock()
}

func (s *darwinPacketSidecar) recordDecodeError() {
	s.statsMu.Lock()
	s.stats.decodeErrors++
	s.statsMu.Unlock()
}

func (s *darwinPacketSidecar) recordAmbiguous() {
	s.statsMu.Lock()
	s.stats.ambiguous++
	s.statsMu.Unlock()
}

func (s *darwinPacketSidecar) recordUnmatched() {
	s.statsMu.Lock()
	s.stats.unmatched++
	s.statsMu.Unlock()
}

func (s *darwinPacketSidecar) publishMatchRate() {
	darwinPacketMatchRate.Set(packetMatchRate(s.snapshot()))
}

func (s *darwinPacketSidecar) remove(cookie uint64) {
	s.analyzer.remove(cookie)
}

func (s *darwinPacketSidecar) stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.source.Close()
	})
	s.wg.Wait()
}

func (s *darwinPacketSidecar) setFailureCallback(callback func(error)) {
	s.onFailure = callback
}

func validateDarwinPacketSidecar(sidecar *darwinPacketSidecar) error {
	if sidecar == nil || sidecar.source == nil || sidecar.primary == nil || sidecar.analyzer == nil {
		return errors.New("incomplete Darwin packet sidecar")
	}
	if sidecar.analyzer.maxFlows <= 0 {
		return fmt.Errorf("invalid Darwin packet flow limit %d", sidecar.analyzer.maxFlows)
	}
	return nil
}
