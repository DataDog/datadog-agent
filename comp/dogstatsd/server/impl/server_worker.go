// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

var (
	// defaultSampleSize is the default allocation size used to store
	// samples when a message contains multiple values. This will
	// automatically be extended if needed when we append to it.
	defaultSampleSize = 128
)

type worker struct {
	server *dsdServer
	// the batcher will be responsible of batching a few samples / events / service
	// checks and it will automatically forward them to the aggregator, meaning that
	// the flushing logic to the aggregator is actually in the batcher.
	batcher         *batcher
	parser          *parser
	identityBuilder *identity.Builder

	// we allocate it once per worker instead of once per packet. This will
	// be used to store the samples out a of packets. Allocating it every
	// time is very costly, especially on the GC.
	samples metrics.MetricSampleBatch

	packetsTelemetry  *packets.TelemetryStore
	packetLog         *packetIngressLog
	rawIngress        packets.RawIngressReader
	columnarV3        aggregator.DogStatsDColumnarV3Inserter
	columnarV3Enabled bool

	FilterListUpdate chan utilstrings.Matcher
	filterList       utilstrings.Matcher
}

func newWorker(s *dsdServer, workerNum int, wmeta option.Option[workloadmeta.Component], packetsTelemetry *packets.TelemetryStore, stringInternerTelemetry *stringInternerTelemetry, filterList utilstrings.Matcher) *worker {
	var batcher *batcher
	if s.ServerlessMode {
		batcher = newServerlessBatcher(s.demultiplexer, s.tlmChannel)
	} else {
		batcher = newBatcher(s.demultiplexer.(aggregator.DemultiplexerWithAggregator), s.tlmChannel)
	}

	var columnarV3 aggregator.DogStatsDColumnarV3Inserter
	columnarV3Enabled := false
	if inserter, ok := s.demultiplexer.(aggregator.DogStatsDColumnarV3Inserter); ok && inserter.DogStatsDColumnarV3Enabled() {
		columnarV3 = inserter
		columnarV3Enabled = true
	}

	return &worker{
		server:            s,
		batcher:           batcher,
		parser:            newParser(s.config, s.sharedFloat64List, workerNum, wmeta, stringInternerTelemetry),
		identityBuilder:   identity.NewBuilder(),
		samples:           make(metrics.MetricSampleBatch, 0, defaultSampleSize),
		packetsTelemetry:  packetsTelemetry,
		columnarV3:        columnarV3,
		columnarV3Enabled: columnarV3Enabled,
		FilterListUpdate:  make(chan utilstrings.Matcher),
		filterList:        filterList,
	}
}

func (w *worker) run() {
	if w.rawIngress != nil {
		w.runRawIngress()
		return
	}
	if w.packetLog != nil {
		w.runPacketLog()
		return
	}
	for {
		select {
		case <-w.server.stopChan:
			return
		case <-w.server.health.C:
		case <-w.server.serverlessFlushChan:
			w.batcher.flush()
		case filterList := <-w.FilterListUpdate:
			w.filterList = filterList
		case ps := <-w.server.packetsIn:
			w.processPackets(ps)
		}

	}
}

func (w *worker) runPacketLog() {
	for {
		select {
		case <-w.server.stopChan:
			return
		case <-w.server.health.C:
		case <-w.server.serverlessFlushChan:
			w.batcher.flush()
		case filterList := <-w.FilterListUpdate:
			w.filterList = filterList
		case <-w.packetLog.notify:
			for {
				ps, ok := w.packetLog.tryNext()
				if !ok {
					break
				}
				w.processPackets(ps)
			}
		}
	}
}

func (w *worker) runRawIngress() {
	for {
		select {
		case <-w.server.stopChan:
			return
		case <-w.server.health.C:
		case <-w.server.serverlessFlushChan:
			w.batcher.flush()
		case filterList := <-w.FilterListUpdate:
			w.filterList = filterList
		case <-w.rawIngress.Notify():
			for {
				rawPacket, ok := w.rawIngress.TryNext()
				if !ok {
					break
				}
				w.processRawPacket(rawPacket)
			}
			w.batcher.flush()
		}
	}
}

func (w *worker) processPackets(ps packets.Packets) {
	w.packetsTelemetry.TelemetryUntrackPackets(ps)
	w.samples = w.samples[0:0]
	// we return the samples in case the slice was extended
	// when parsing the packets
	w.samples = w.server.parsePackets(w.batcher, w.parser, w.identityBuilder, ps, w.samples, &w.filterList)
}

func (w *worker) processRawPacket(rawPacket packets.RawPacket) {
	packet := packets.Packet{
		Contents:   rawPacket.Contents,
		Origin:     rawPacket.Origin,
		ProcessID:  rawPacket.ProcessID,
		ListenerID: rawPacket.ListenerID,
		Source:     rawPacket.Source,
	}
	w.samples = w.samples[0:0]
	w.samples = w.server.parsePacket(w.batcher, w.parser, w.identityBuilder, &packet, w.samples, &w.filterList, w.columnarV3, w.columnarV3Enabled)
	rawPacket.Release()
}
